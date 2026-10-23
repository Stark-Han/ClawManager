package systembackup

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func verifyFixtureRequest() VerifyJobRequest {
	return VerifyJobRequest{Namespace: "restore-fixture", NamespaceUID: "namespace-uid", Image: "registry.invalid/backup@sha256:" + strings.Repeat("a", 64), ServiceAccount: "verifier", BootstrapSecret: "verifier-bootstrap", Origin: "verify-installation", BackupID: "sbk_" + strings.Repeat("1", 32), MaxBytes: 1 << 20, DeadlineSeconds: 60}
}
func TestVerifyJobAdoptionCollisionAndBoundResult(t *testing.T) {
	ctx := context.Background()
	request := verifyFixtureRequest()
	raw, _ := json.Marshal(request)
	a := Attempt{ID: "sat_" + strings.Repeat("2", 32), TaskID: "sav_" + strings.Repeat("3", 32), Request: raw}
	client := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: request.Namespace, UID: types.UID(request.NamespaceUID)}})
	j := VerifyJobs{Client: client}
	created, e := j.Ensure(ctx, a)
	if e != nil {
		t.Fatal(e)
	}
	created.UID = "job-uid"
	created.Status.Conditions = []batchv1.JobCondition{{Type: batchv1.JobComplete, Status: corev1.ConditionTrue}}
	if _, e = client.BatchV1().Jobs(request.Namespace).Update(ctx, created, metav1.UpdateOptions{}); e != nil {
		t.Fatal(e)
	}
	if _, e = j.Ensure(ctx, a); e != nil {
		t.Fatal("lost-response adoption", e)
	}
	list, _ := client.BatchV1().Jobs(request.Namespace).List(ctx, metav1.ListOptions{})
	if len(list.Items) != 1 {
		t.Fatal("duplicate execution")
	}
	if _, e = j.Observe(ctx, a); e == nil {
		t.Fatal("exit status accepted without bounded report")
	}
	yes := true
	report := IntegrityReport{BackupID: request.BackupID, Origin: request.Origin, Purpose: "functional_test", Consistency: "best_effort", IntegrityVerified: true, PartsVerified: 6}
	message, _ := json.Marshal(report)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "verify-pod", Namespace: request.Namespace, Labels: created.Labels, OwnerReferences: []metav1.OwnerReference{{UID: created.UID, Kind: "Job", Controller: &yes}}}, Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{{Name: "verify", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0, Message: string(message)}}}}}}
	if _, e = client.CoreV1().Pods(request.Namespace).Create(ctx, pod, metav1.CreateOptions{}); e != nil {
		t.Fatal(e)
	}
	result, e := j.Observe(ctx, a)
	if e != nil || result.State != "succeeded" || result.Report.AcceptanceEligible {
		t.Fatal(result, e)
	}
	// A matching annotation does not authorize a substituted command/image.
	created.Spec.Template.Spec.Containers[0].Command = []string{"/bin/sh"}
	client.BatchV1().Jobs(request.Namespace).Update(ctx, created, metav1.UpdateOptions{})
	if _, e = j.Ensure(ctx, a); e == nil {
		t.Fatal("mutated Job adopted")
	}
	if _, e = j.Stop(ctx, a); e == nil {
		t.Fatal("foreign Job deleted")
	}
}

type observedJobs struct {
	ids    []string
	result VerifyJobResult
	stop   bool
}

func (j *observedJobs) Observe(_ context.Context, a Attempt) (VerifyJobResult, error) {
	j.ids = append(j.ids, a.ID)
	return j.result, nil
}
func (j *observedJobs) Stop(_ context.Context, a Attempt) (bool, error) {
	j.ids = append(j.ids, a.ID)
	return j.stop, nil
}
func TestDurableVerificationControllerAdoptsAttemptAfterLeaderLoss(t *testing.T) {
	ctx := context.Background()
	s := Store{DB: controlDB(t), InstallationID: "verify-installation"}
	if e := s.Bootstrap(ctx); e != nil {
		t.Fatal(e)
	}
	if _, e := s.SaveConfig(ctx, 0, 1, testConfig(), permitConfig); e != nil {
		t.Fatal(e)
	}
	task, _, e := s.Create(ctx, 1, "verify-adoption-test", CreateTaskRequest{Type: ArtifactVerify, Purpose: "functional_test", SourceID: verifyFixtureRequest().BackupID}, permitTask)
	if e != nil {
		t.Fatal(e)
	}
	jobs := &observedJobs{result: VerifyJobResult{State: "running", JobUID: "uid"}}
	runner := &VerifyTaskRunner{Jobs: jobs, Template: verifyFixtureRequest()}
	c := Controller{Store: s, Owner: "first-controller", PollInterval: time.Second, Verifier: runner}
	if e = c.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	a, e := s.GetAttempt(ctx, task.ID, "running")
	if e != nil {
		t.Fatal(e)
	}
	if len(jobs.ids) != 1 || jobs.ids[0] != a.ID {
		t.Fatal("missing frozen intent")
	}
	first, e := s.AcquireLeadership(ctx, c.Owner, 30*time.Second)
	if e != nil {
		t.Fatal(e)
	}
	oldTask, _ := s.Get(ctx, task.ID)
	if _, e = s.DB.Exec("UPDATE system_backup_controller_leases SET expires_at=TIMESTAMPADD(SECOND,-1,UTC_TIMESTAMP(6))"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB.Exec("UPDATE system_backup_tasks SET lease_until=TIMESTAMPADD(SECOND,-1,UTC_TIMESTAMP(6)) WHERE public_id=?", task.ID); e != nil {
		t.Fatal(e)
	}
	c.Owner = "replacement-controller"
	jobs.result = VerifyJobResult{State: "succeeded", JobUID: "uid", Report: &IntegrityReport{BackupID: task.SourceID, Origin: s.InstallationID, IntegrityVerified: true, PartsVerified: 6}}
	if e = c.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	finished, e := s.Get(ctx, task.ID)
	if e != nil || finished.Status != "succeeded" || finished.Eligible || !finished.DeadlineAt.Equal(task.DeadlineAt) {
		t.Fatal(finished, e)
	}
	if len(jobs.ids) != 2 || jobs.ids[0] != jobs.ids[1] {
		t.Fatal("takeover launched a second attempt")
	}
	if e = s.FinishAttempt(ctx, first, oldTask, a, "failed", map[string]string{"reason": "stale"}); !errors.Is(e, ErrFence) {
		t.Fatal("stale leader changed result", e)
	}
	var count int
	if e = s.DB.QueryRow("SELECT COUNT(*) FROM system_backup_attempts WHERE task_id=?", task.ID).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
	// Running cancellation waits for physical Job deletion confirmation.
	second, _, e := s.Create(ctx, 1, "verify-cancel-test", CreateTaskRequest{Type: ArtifactVerify, Purpose: "functional_test", SourceID: task.SourceID}, permitTask)
	if e != nil {
		t.Fatal(e)
	}
	jobs.result = VerifyJobResult{State: "running"}
	if e = c.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	second, _ = s.Get(ctx, second.ID)
	if _, e = s.Cancel(ctx, second.ID, second.Version, 1); e != nil {
		t.Fatal(e)
	}
	if e = c.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	second, _ = s.Get(ctx, second.ID)
	if second.Status != "canceling" {
		t.Fatal("canceled before Job stopped")
	}
	jobs.stop = true
	if e = c.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	second, _ = s.Get(ctx, second.ID)
	if second.Status != "canceled" {
		t.Fatal(second)
	}
}
