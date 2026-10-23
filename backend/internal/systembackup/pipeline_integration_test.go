package systembackup

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"clawreef/internal/systembackup/team"
	"github.com/minio/minio-go/v7"
)

func TestSixRealSourcesPublishAndDataOnlyRestore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	source := realKubernetesCaptureSources(t)
	source.MySQL = fullSchemaDB(t)
	source.Redis = redisFixture(t, "TEST_SYSTEM_BACKUP_REDIS_PORT")
	sourceObjects := testObjectStore(t)
	external := testObjectStore(t)
	source.Objects = sourceObjects.Client
	source.ObjectBuckets = []string{sourceObjects.Bucket}
	root := t.TempDir()
	stage := t.TempDir()
	source.Workspace = []FileRoot{{ID: "owner_x", Path: root}}
	payload := []byte("interrupted task artifact\x00\xff\n")
	if e := os.MkdirAll(filepath.Join(root, ".state"), 0700); e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{"partial.bin", ".state/active-assignment.json"} {
		if e := os.WriteFile(filepath.Join(root, p), payload, 0600); e != nil {
			t.Fatal(e)
		}
	}
	for _, q := range []string{`INSERT INTO users(id,username,email,password_hash,role) VALUES(901,'backup-provider','backup@test.invalid','disabled','user')`, `INSERT INTO teams(id,user_id,name,status,resource_scope,owner) VALUES(901,901,'backup-owner','running','northbound','owner-x')`, `INSERT INTO team_members(id,team_id,user_id,member_key,display_name,role,status) VALUES(901,901,901,'leader','Leader','leader','busy')`, `INSERT INTO team_tasks(id,team_id,target_member_id,message_id,status,payload_json,result_json,workflow_state) VALUES(901,901,901,'backup-task','running','{}','{"partial":"preserve this result"}','executing')`} {
		if _, e := source.MySQL.ExecContext(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := source.Objects.PutObject(ctx, sourceObjects.Bucket, "team-901/partial.bin", bytes.NewReader(payload), int64(len(payload)), minio.PutObjectOptions{}); e != nil {
		t.Fatal(e)
	}
	redisKey, _ := RandomID("pipeline_")
	key := []byte(redisKey)
	if _, e := source.Redis.command(ctx, []byte("SET"), key, payload); e != nil {
		t.Fatal(e)
	}
	run, _ := RandomID("sdr_")
	redisTarget := redisFixture(t, "TEST_SYSTEM_BACKUP_REDIS_TARGET_PORT")
	t.Cleanup(func() {
		source.Redis.command(context.Background(), []byte("DEL"), key)
		redisTarget.command(context.Background(), []byte("DEL"), []byte("cm:restore-history:"+run+":"+hex.EncodeToString(key)))
	})
	id, _ := RandomID("sbk_")
	kek := make([]byte, 32)
	rand.Read(kek)
	wrapper := AESKeyWrapper{SecretRef{"kubernetes", "pipeline-key", "v1", "encryption"}, kek}
	pub, signer, _ := ed25519.GenerateKey(rand.Reader)
	signed, e := CaptureFunctionalBundle(ctx, stage, "fixture-installation", id, source, CaptureBudget{64 << 20, 10000}, wrapper, signer, "v1")
	if e != nil {
		t.Fatal("six-part capture:", e)
	}
	published, e := PublishFunctionalArtifact(ctx, external, stage, signed, signer, "v1", 64<<20)
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range RequiredParts {
		if e = os.Remove(filepath.Join(stage, name+".enc")); e != nil {
			t.Fatal(e)
		}
	}
	// From here no source adapter is supplied to any restoration function.
	recovered, e := OpenPublishedArtifact(ctx, external, "fixture-installation", id, pub, "v1")
	if e != nil {
		t.Fatal(e)
	}
	if e = VerifyPublishedArtifact(ctx, external, recovered, wrapper, 64<<20); e != nil {
		t.Fatal(e)
	}
	target, targetName := isolatedSchemaDB(t, source.MySQL)
	var sourceName string
	source.MySQL.QueryRow("SELECT DATABASE()").Scan(&sourceName)
	control := Store{DB: controlDB(t), InstallationID: "restore-controller"}
	targetHash, _ := RequestHash(targetName)
	at := time.Now().UTC()
	evidence := func(table SQLTable, change team.Change) error {
		if !change.Quarantined {
			return nil
		}
		_, e := control.RecordRestoreEvidence(ctx, run, id, targetHash, table, change, wrapper)
		return e
	}
	if e = ConsumePublishedPart(ctx, external, recovered, "mysql", wrapper, 64<<20, func(r io.Reader) error {
		_, e := RestoreMySQL(ctx, target, sourceName, targetName, r, func(table SQLTable, row team.Row) (team.Row, error) {
			change, e := NormalizeDataRow(table, row, at)
			if e != nil {
				return nil, e
			}
			if e = evidence(table, change); e != nil {
				return nil, e
			}
			return change.After, nil
		})
		return e
	}); e != nil {
		t.Fatal("SQL restore:", e)
	}
	var normalized bytes.Buffer
	if e = ConsumePublishedPart(ctx, external, recovered, "mysql", wrapper, 64<<20, func(r io.Reader) error { _, e := NormalizeSQLSnapshot(ctx, r, &normalized, at, evidence); return e }); e != nil {
		t.Fatal(e)
	}
	verifier := selectOnlyVerifier(t, source.MySQL, targetName)
	if _, e = VerifyMySQL(ctx, verifier, bytes.NewReader(normalized.Bytes()), 100000); e != nil {
		t.Fatal(e)
	}
	var status, result string
	if e = verifier.QueryRow("SELECT status,result_json FROM team_tasks WHERE id=901").Scan(&status, &result); e != nil || status != "failed" || result != "{\"partial\":\"preserve this result\"}" {
		t.Fatal("interrupted task data lost", status, e)
	}
	workspaceTarget := t.TempDir()
	if e = ConsumePublishedPart(ctx, external, recovered, "workspace", wrapper, 64<<20, func(r io.Reader) error {
		_, e := ExtractFiles(ctx, r, workspaceTarget, FileBudget{64 << 20, 10000})
		return e
	}); e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile(filepath.Join(workspaceTarget, "roots", "owner_x", "partial.bin"))
	if e != nil || !bytes.Equal(got, payload) {
		t.Fatal("workspace artifact lost", e)
	}
	if e = ConsumePublishedPart(ctx, external, recovered, "redis", wrapper, 64<<20, func(r io.Reader) error {
		return RestoreRedisHistory(ctx, redisTarget, r, RedisBudget{MaxBytes: 64 << 20, MaxItems: 10000, MaxInventoryBytes: 8 << 20}, run)
	}); e != nil {
		t.Fatal(e)
	}
	if n, e := redisTarget.command(ctx, []byte("EXISTS"), key); e != nil || n != int64(0) {
		t.Fatal("old Redis task namespace reactivated")
	}
	targetBucket, _ := RandomID("cm-restore-")
	if e = source.Objects.MakeBucket(ctx, targetBucket, minio.MakeBucketOptions{}); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		for o := range source.Objects.ListObjects(context.Background(), targetBucket, minio.ListObjectsOptions{Recursive: true}) {
			if o.Err == nil {
				source.Objects.RemoveObject(context.Background(), targetBucket, o.Key, minio.RemoveObjectOptions{})
			}
		}
		source.Objects.RemoveBucket(context.Background(), targetBucket)
	})
	if e = ConsumePublishedPart(ctx, external, recovered, "object_storage", wrapper, 64<<20, func(r io.Reader) error {
		return RestoreObjects(ctx, source.Objects, r, ObjectBudget{64 << 20, 10000}, map[string]string{sourceObjects.Bucket: targetBucket})
	}); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"deployment_resources", "secret_bundle"} {
		count := 0
		if e = ConsumePublishedPart(ctx, external, recovered, name, wrapper, 64<<20, func(r io.Reader) error {
			return ReadKubernetesHistory(ctx, r, name, ObjectBudget{64 << 20, 10000}, func(_ KubernetesSelection, _ json.RawMessage) error { count++; return nil })
		}); e != nil || count == 0 {
			t.Fatal("resource history lost", name, e)
		}
	}
	if published.Manifest.Payload.Consistency != "best_effort" || published.Manifest.Payload.Purpose != "functional_test" {
		t.Fatal("unproven strong backup claimed")
	}
}
