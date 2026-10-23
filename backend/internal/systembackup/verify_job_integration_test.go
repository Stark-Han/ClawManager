package systembackup

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Enabled in the six-source integration test only when a disposable namespace,
// restricted controller identity and digest-pinned executable image are given.
func runRealVerifyJob(t *testing.T, ctx context.Context, store ImmutableObjectStore, artifact PublishedArtifact, wrapper AESKeyWrapper, pub ed25519.PublicKey) {
	t.Helper()
	path := os.Getenv("TEST_SYSTEM_BACKUP_JOB_AUTH_FILE")
	if path == "" {
		return
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal("Job fixture credentials unavailable")
	}
	defer clear(raw)
	var cfg struct {
		Namespace       string `json:"namespace"`
		NamespaceUID    string `json:"namespace_uid"`
		CA              string `json:"ca"`
		Token           string `json:"token"`
		ArchiveCA       string `json:"archive_ca"`
		ArchiveEndpoint string `json:"archive_endpoint"`
		User            string `json:"verify_user"`
		Password        string `json:"verify_password"`
	}
	if json.Unmarshal(raw, &cfg) != nil {
		t.Fatal("Job fixture credentials invalid")
	}
	client, e := kubernetes.NewForConfig(&rest.Config{Host: "https://127.0.0.1:36443", BearerToken: cfg.Token, Timeout: 10 * time.Second, TLSClientConfig: rest.TLSClientConfig{CAData: []byte(cfg.CA), ServerName: "10.130.14.23"}})
	if e != nil {
		t.Fatal(e)
	}
	n, e := client.CoreV1().Namespaces().Get(ctx, cfg.Namespace, metav1.GetOptions{})
	if e != nil || n.Labels["backup-test-owner"] != "cm-20261023-1209" || string(n.UID) != cfg.NamespaceUID {
		t.Fatal("unowned Job fixture")
	}
	// The actual artifact reader is denied writes by object-store permissions.
	reader, e := minio.New("127.0.0.1:"+os.Getenv("TEST_SYSTEM_BACKUP_MINIO_PORT"), &minio.Options{Creds: credentials.NewStaticV4(cfg.User, cfg.Password, ""), Secure: false})
	if e != nil {
		t.Fatal(e)
	}
	if e = reader.RemoveObject(ctx, store.Bucket, "permission-probe", minio.RemoveObjectOptions{}); minio.ToErrorResponse(e).Code != "AccessDenied" {
		t.Fatal("verification identity not read-only")
	}
	encode := func(v any) []byte {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		return b
	}
	secretName, _ := RandomID("backup-verify-")
	yes := true
	data := map[string][]byte{"verify.pub": pub, "encryption.key": wrapper.Key, "archive-ca.pem": []byte(cfg.ArchiveCA), "credentials.json": encode(map[string]string{"access_key": cfg.User, "secret_key": cfg.Password})}
	data["config.json"] = encode(map[string]any{"endpoint": cfg.ArchiveEndpoint, "bucket": store.Bucket, "root": store.Root, "secure": true, "credentials_file": "/run/backup-bootstrap/credentials.json", "ca_file": "/run/backup-bootstrap/archive-ca.pem", "verification_key_file": "/run/backup-bootstrap/verify.pub", "verification_key_version": "v1", "encryption_key_file": "/run/backup-bootstrap/encryption.key", "encryption_reference": wrapper.Reference})
	secret, e := client.CoreV1().Secrets(cfg.Namespace).Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName}, Immutable: &yes, Data: data}, metav1.CreateOptions{})
	if e != nil {
		t.Fatal("Job fixture secret create failed", e)
	}
	aid, _ := RandomID("sat_")
	tid, _ := RandomID("sav_")
	request := VerifyJobRequest{Namespace: cfg.Namespace, NamespaceUID: cfg.NamespaceUID, Image: os.Getenv("TEST_SYSTEM_BACKUP_JOB_IMAGE"), ServiceAccount: "backup-verifier", BootstrapSecret: secretName, Origin: artifact.Manifest.Payload.OriginInstallationID, BackupID: artifact.Manifest.Payload.BackupID, MaxBytes: 64 << 20, DeadlineSeconds: 90}
	a := Attempt{ID: aid, TaskID: tid, Request: encode(request)}
	jobs := VerifyJobs{Client: client}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for {
			stopped, e := jobs.Stop(cleanup, a)
			if e != nil {
				t.Error("Job cleanup failed", e)
				break
			}
			if stopped {
				break
			}
			select {
			case <-cleanup.Done():
				t.Error("Job cleanup deadline")
				return
			case <-time.After(time.Second):
			}
		}
		uid := secret.UID
		if e := client.CoreV1().Secrets(cfg.Namespace).Delete(cleanup, secretName, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); e != nil {
			t.Error("fixture credential cleanup failed", e)
		}
	})
	first, e := jobs.Ensure(ctx, a)
	if e != nil {
		t.Fatal("real Job create failed", e)
	}
	// Reconstructing the driver models process replacement after lost response.
	adopted, e := (VerifyJobs{Client: client}).Ensure(ctx, a)
	if e != nil || adopted.UID != first.UID {
		t.Fatal("real Job adoption failed", e)
	}
	for {
		result, e := jobs.Observe(ctx, a)
		if e != nil {
			t.Fatal("real verification Job failed", e)
		}
		if result.State == "failed" {
			t.Fatal("verification executable failed in isolated Job")
		}
		if result.State == "succeeded" {
			if result.Report.AcceptanceEligible {
				t.Fatal("integrity verify fabricated acceptance")
			}
			t.Log("real Kubernetes verification Job: six parts decrypted, read-only object identity, immutable Secret, digest image, adopted UID, bounded report")
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("real Job deadline")
		case <-time.After(time.Second):
		}
	}
}
