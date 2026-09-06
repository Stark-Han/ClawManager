package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"clawreef/internal/config"
	"clawreef/internal/models"
)

type scriptedUpgradeLabRoot struct {
	removeErrors []error
	lstatErrors  []error
	removeCalls  int
	lstatCalls   int
}

func (r *scriptedUpgradeLabRoot) RemoveAll(string) error {
	index := r.removeCalls
	r.removeCalls++
	if index < len(r.removeErrors) {
		return r.removeErrors[index]
	}
	return nil
}

func (r *scriptedUpgradeLabRoot) Lstat(string) (os.FileInfo, error) {
	index := r.lstatCalls
	r.lstatCalls++
	if index < len(r.lstatErrors) {
		return nil, r.lstatErrors[index]
	}
	return nil, os.ErrNotExist
}

type upgradeLabCleanupInstanceRepo struct {
	*fakeRuntimeInstanceRepo
	deleteCalls []int
	events      *[]string
}

func (r *upgradeLabCleanupInstanceRepo) Delete(id int) error {
	r.deleteCalls = append(r.deleteCalls, id)
	if r.events != nil {
		*r.events = append(*r.events, "instance")
	}
	delete(r.byID, id)
	return nil
}

type upgradeLabCleanupDeployments struct {
	deleted []string
	events  *[]string
}

func (*upgradeLabCleanupDeployments) EnsureUpgradeLabPool(context.Context, string, string, string, string, string, int32) error {
	return nil
}

func (d *upgradeLabCleanupDeployments) DeleteUpgradeLabPool(_ context.Context, _, name, _ string) error {
	d.deleted = append(d.deleted, name)
	if d.events != nil {
		*d.events = append(*d.events, name)
	}
	return nil
}

type upgradeLabCleanupAgent struct {
	*fakeRuntimeAgentClient
	stateCalls int
}

func (a *upgradeLabCleanupAgent) GatewayState(context.Context, string, string) (*RuntimeAgentGatewayState, error) {
	a.stateCalls++
	return nil, ErrRuntimeAgentNotFound
}

func TestRemoveUpgradeLabWorkspaceRetriesTransientNFSNotEmpty(t *testing.T) {
	root := &scriptedUpgradeLabRoot{
		removeErrors: []error{errors.New("unlinkat workspace: directory not empty"), nil},
		lstatErrors:  []error{os.ErrNotExist, os.ErrNotExist},
	}
	if err := removeUpgradeLabWorkspaceWithRetry(context.Background(), root, "openclaw/user-1/instance-203", time.Second, time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if root.removeCalls != 2 {
		t.Fatalf("RemoveAll calls = %d, want 2", root.removeCalls)
	}
}

func TestRemoveUpgradeLabWorkspaceDoesNotRetryPermissionFailure(t *testing.T) {
	root := &scriptedUpgradeLabRoot{removeErrors: []error{os.ErrPermission}}
	err := removeUpgradeLabWorkspaceWithRetry(context.Background(), root, "openclaw/user-1/instance-203", time.Second, time.Millisecond)
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("cleanup error = %v, want permission failure", err)
	}
	if root.removeCalls != 1 {
		t.Fatalf("RemoveAll calls = %d, want 1", root.removeCalls)
	}
}

func TestRemoveLabWorkspaceIsRootedAndRemovesNestedFixture(t *testing.T) {
	root := t.TempDir()
	workspace := RuntimeWorkspacePathWithRoot(root, RuntimeTypeOpenClaw, 1, 203)
	if err := os.MkdirAll(filepath.Join(workspace, "home", ".openclaw"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "home", ".openclaw", "fixture"), []byte("test"), 0o640); err != nil {
		t.Fatal(err)
	}
	service := &OpenClawUpgradeLabService{cfg: config.RuntimePoolConfig{WorkspaceRoot: root}}
	if err := service.removeLabWorkspace(context.Background(), workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(workspace); !os.IsNotExist(err) {
		t.Fatalf("workspace still exists: %v", err)
	}
	if err := service.removeLabWorkspace(context.Background(), root); err == nil {
		t.Fatal("cleanup accepted the workspace root")
	}
}

func TestDeleteLabInstanceKeepsDatabaseRowUntilWorkspaceCleanupSucceeds(t *testing.T) {
	root := t.TempDir()
	workspace := RuntimeWorkspacePathWithRoot(root, RuntimeTypeOpenClaw, 1, 203)
	description := openClawUpgradeLabDescription + "4"
	instance := &models.Instance{ID: 203, UserID: 1, Description: &description, WorkspacePath: &workspace}
	instances := &upgradeLabCleanupInstanceRepo{fakeRuntimeInstanceRepo: newFakeRuntimeInstanceRepo()}
	instances.byID[instance.ID] = instance
	service := &OpenClawUpgradeLabService{
		instances: instances,
		bindings:  newFakeRuntimeBindingRepo(),
		cfg:       config.RuntimePoolConfig{WorkspaceRoot: root},
		workspaceRemover: func(context.Context, string) error {
			return errors.New("unlinkat workspace: directory not empty")
		},
	}
	if err := service.deleteLabInstance(context.Background(), instance); err == nil {
		t.Fatal("cleanup unexpectedly succeeded")
	}
	if len(instances.deleteCalls) != 0 || instances.byID[instance.ID] == nil {
		t.Fatal("instance row was deleted before workspace cleanup succeeded")
	}
	service.workspaceRemover = func(context.Context, string) error { return nil }
	if err := service.deleteLabInstance(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	if len(instances.deleteCalls) != 1 || instances.byID[instance.ID] != nil {
		t.Fatal("instance row was not deleted after workspace cleanup succeeded")
	}
}

func TestDeleteLabInstanceConfirmsGatewayStopBeforeWorkspaceCleanup(t *testing.T) {
	root := t.TempDir()
	workspace := RuntimeWorkspacePathWithRoot(root, RuntimeTypeOpenClaw, 1, 204)
	description := openClawUpgradeLabDescription + "5"
	endpoint := "http://runtime-agent"
	instance := &models.Instance{ID: 204, UserID: 1, Description: &description, WorkspacePath: &workspace}
	instances := &upgradeLabCleanupInstanceRepo{fakeRuntimeInstanceRepo: newFakeRuntimeInstanceRepo()}
	instances.byID[instance.ID] = instance
	bindings := newFakeRuntimeBindingRepo()
	bindings.bindings[instance.ID] = &models.InstanceRuntimeBinding{InstanceID: instance.ID, RuntimePodID: 9, GatewayID: "gateway-204"}
	agent := &upgradeLabCleanupAgent{fakeRuntimeAgentClient: &fakeRuntimeAgentClient{}}
	workspaceRemoved := false
	service := &OpenClawUpgradeLabService{
		instances: instances,
		pods:      &fakeRuntimePodRepo{pods: map[int64]*models.RuntimePod{9: {ID: 9, AgentEndpoint: &endpoint}}},
		bindings:  bindings,
		agent:     agent,
		cfg:       config.RuntimePoolConfig{WorkspaceRoot: root},
		workspaceRemover: func(context.Context, string) error {
			if agent.stateCalls == 0 {
				t.Fatal("workspace cleanup ran before gateway stop confirmation")
			}
			workspaceRemoved = true
			return nil
		},
	}
	if err := service.deleteLabInstance(context.Background(), instance); err != nil {
		t.Fatal(err)
	}
	if agent.stateCalls == 0 || !workspaceRemoved || bindings.bindings[instance.ID] != nil || instances.byID[instance.ID] != nil {
		t.Fatalf("incomplete cleanup: stateCalls=%d workspaceRemoved=%v binding=%v instance=%v", agent.stateCalls, workspaceRemoved, bindings.bindings[instance.ID], instances.byID[instance.ID])
	}
}

func TestDeleteLabInstanceDoesNotReleaseUnaddressableGatewayBinding(t *testing.T) {
	root := t.TempDir()
	workspace := RuntimeWorkspacePathWithRoot(root, RuntimeTypeOpenClaw, 1, 206)
	description := openClawUpgradeLabDescription + "7"
	instance := &models.Instance{ID: 206, UserID: 1, Description: &description, WorkspacePath: &workspace}
	instances := &upgradeLabCleanupInstanceRepo{fakeRuntimeInstanceRepo: newFakeRuntimeInstanceRepo()}
	instances.byID[instance.ID] = instance
	bindings := newFakeRuntimeBindingRepo()
	bindings.bindings[instance.ID] = &models.InstanceRuntimeBinding{InstanceID: instance.ID, RuntimePodID: 12, GatewayID: "gateway-206"}
	workspaceRemoved := false
	service := &OpenClawUpgradeLabService{
		instances: instances,
		pods:      &fakeRuntimePodRepo{pods: map[int64]*models.RuntimePod{}},
		bindings:  bindings,
		agent:     &fakeRuntimeAgentClient{},
		cfg:       config.RuntimePoolConfig{WorkspaceRoot: root},
		workspaceRemover: func(context.Context, string) error {
			workspaceRemoved = true
			return nil
		},
	}
	if err := service.deleteLabInstance(context.Background(), instance); err == nil || !strings.Contains(err.Error(), "addressable") {
		t.Fatalf("unaddressable cleanup error = %v", err)
	}
	if bindings.bindings[instance.ID] == nil || instances.byID[instance.ID] == nil || workspaceRemoved {
		t.Fatal("unaddressable gateway cleanup released durable state")
	}
}

func TestCleanupRunResourcesDeletesIsolatedPoolsBeforeWorkspaceAndRow(t *testing.T) {
	root := t.TempDir()
	workspace := RuntimeWorkspacePathWithRoot(root, RuntimeTypeOpenClaw, 1, 203)
	description := openClawUpgradeLabDescription + "4"
	instance := &models.Instance{ID: 203, UserID: 1, Description: &description, WorkspacePath: &workspace}
	events := []string{}
	instances := &upgradeLabCleanupInstanceRepo{fakeRuntimeInstanceRepo: newFakeRuntimeInstanceRepo(), events: &events}
	instances.byID[instance.ID] = instance
	deployments := &upgradeLabCleanupDeployments{events: &events}
	service := &OpenClawUpgradeLabService{
		instances:   instances,
		pods:        &fakeRuntimePodRepo{pods: map[int64]*models.RuntimePod{}},
		bindings:    newFakeRuntimeBindingRepo(),
		deployments: deployments,
		cfg:         config.RuntimePoolConfig{Namespace: "test", WorkspaceRoot: root},
		workspaceRemover: func(context.Context, string) error {
			events = append(events, "workspace")
			return nil
		},
	}
	actorID := 1
	rolloutID := int64(52)
	instanceIDs := "[203]"
	run := &models.OpenClawUpgradeLabRun{ID: 4, ActorUserID: &actorID, SourceDeployment: "openclaw-upgrade-lab-r4-source", RolloutID: &rolloutID, InstanceIDsJSON: &instanceIDs}
	if err := service.cleanupRunResources(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	want := []string{"openclaw-upgrade-lab-r4-source-u52", "openclaw-upgrade-lab-r4-source", "workspace", "instance"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("cleanup order = %v, want %v", events, want)
	}
}

func TestCleanupRunResourcesRecoversMissingInstanceAfterPoolsStop(t *testing.T) {
	root := t.TempDir()
	events := []string{}
	instances := &upgradeLabCleanupInstanceRepo{fakeRuntimeInstanceRepo: newFakeRuntimeInstanceRepo(), events: &events}
	deployments := &upgradeLabCleanupDeployments{events: &events}
	service := &OpenClawUpgradeLabService{
		instances:   instances,
		pods:        &fakeRuntimePodRepo{pods: map[int64]*models.RuntimePod{}},
		bindings:    newFakeRuntimeBindingRepo(),
		deployments: deployments,
		cfg:         config.RuntimePoolConfig{Namespace: "test", WorkspaceRoot: root},
		workspaceRemover: func(_ context.Context, target string) error {
			events = append(events, "workspace")
			want := RuntimeWorkspacePathWithRoot(root, RuntimeTypeOpenClaw, 1, 203)
			if !sameCleanPath(target, want) {
				t.Fatalf("orphan workspace = %q, want %q", target, want)
			}
			return nil
		},
	}
	actorID := 1
	rolloutID := int64(52)
	instanceIDs := "[203]"
	run := &models.OpenClawUpgradeLabRun{ID: 4, ActorUserID: &actorID, SourceDeployment: "openclaw-upgrade-lab-r4-source", RolloutID: &rolloutID, InstanceIDsJSON: &instanceIDs}
	if err := service.cleanupRunResources(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	want := []string{"openclaw-upgrade-lab-r4-source-u52", "openclaw-upgrade-lab-r4-source", "workspace"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("partial cleanup order = %v, want %v", events, want)
	}
}

func TestCleanupRunResourcesRejectsBindingOutsideLabBeforeMutation(t *testing.T) {
	root := t.TempDir()
	workspace := RuntimeWorkspacePathWithRoot(root, RuntimeTypeOpenClaw, 1, 205)
	description := openClawUpgradeLabDescription + "6"
	instance := &models.Instance{ID: 205, UserID: 1, Description: &description, WorkspacePath: &workspace}
	instances := &upgradeLabCleanupInstanceRepo{fakeRuntimeInstanceRepo: newFakeRuntimeInstanceRepo()}
	instances.byID[instance.ID] = instance
	bindings := newFakeRuntimeBindingRepo()
	bindings.bindings[instance.ID] = &models.InstanceRuntimeBinding{InstanceID: instance.ID, RuntimePodID: 11, GatewayID: "gateway-205"}
	deployments := &upgradeLabCleanupDeployments{}
	service := &OpenClawUpgradeLabService{
		instances:   instances,
		pods:        &fakeRuntimePodRepo{pods: map[int64]*models.RuntimePod{11: {ID: 11, DeploymentName: "openclaw-runtime"}}},
		bindings:    bindings,
		deployments: deployments,
		cfg:         config.RuntimePoolConfig{Namespace: "test", WorkspaceRoot: root},
	}
	actorID := 1
	instanceIDs := "[205]"
	run := &models.OpenClawUpgradeLabRun{ID: 6, ActorUserID: &actorID, SourceDeployment: "openclaw-upgrade-lab-r6-source", InstanceIDsJSON: &instanceIDs}
	if err := service.cleanupRunResources(context.Background(), run); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("outside binding cleanup error = %v", err)
	}
	if len(deployments.deleted) != 0 || len(instances.deleteCalls) != 0 {
		t.Fatal("cleanup mutated resources before rejecting an outside binding")
	}
}

func TestInspectUpgradeLabProjectDataIsDeterministicAndDetectsChanges(t *testing.T) {
	workspace := t.TempDir()
	project := filepath.Join(workspace, "project")
	if err := os.MkdirAll(filepath.Join(project, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "b.txt"), []byte("two"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "nested", "a.txt"), []byte("one"), 0o640); err != nil {
		t.Fatal(err)
	}
	first, err := inspectUpgradeLabProjectData(workspace)
	if err != nil {
		t.Fatal(err)
	}
	second, err := inspectUpgradeLabProjectData(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == "" || first.Digest != second.Digest || len(first.Files) != 2 {
		t.Fatalf("non-deterministic manifest: first=%#v second=%#v", first, second)
	}
	if err := os.WriteFile(filepath.Join(project, "b.txt"), []byte("changed"), 0o640); err != nil {
		t.Fatal(err)
	}
	changed, err := inspectUpgradeLabProjectData(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Digest == first.Digest {
		t.Fatal("project mutation was not detected")
	}
}

func TestUpgradeLabErrorRedactionDoesNotExposeCredentials(t *testing.T) {
	for _, input := range []string{
		"request failed token=secret-value more text",
		"Authorization: Bearer top-secret",
		"connector failed api_key=top-secret",
		"plugin credential igt_secret-value",
	} {
		got := redactUpgradeLabError(input)
		if strings.Contains(got, "secret-value") || strings.Contains(got, "top-secret") {
			t.Fatalf("credential remained in %q", got)
		}
		if !strings.Contains(got, "[redacted]") {
			t.Fatalf("redaction marker missing from %q", got)
		}
	}
}

func TestInspectUpgradeLabLegacySessionsRequiresManualConversation(t *testing.T) {
	workspace := t.TempDir()
	sessions := filepath.Join(workspace, "home", ".openclaw", "agents", "main", "sessions")
	if err := os.MkdirAll(sessions, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessions, "sessions.json"), []byte(`{"agent:main:main":{"sessionId":"session-1"}}`), 0o640); err != nil {
		t.Fatal(err)
	}
	transcript := strings.Join([]string{
		`{"type":"message","message":{"role":"user","content":"[OpenClaw heartbeat poll]"}}`,
		`{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"ok"}]}}`,
		`{"type":"message","message":{"role":"user","content":[{"type":"text","text":"请记住这个测试"}]}}`,
		`{"type":"message","message":{"role":"assistant","content":"已记住"}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(sessions, "session-1.jsonl"), []byte(transcript), 0o640); err != nil {
		t.Fatal(err)
	}
	promptCache := filepath.Join(sessions, "skills-prompts", "sha256", "aa")
	if err := os.MkdirAll(promptCache, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(promptCache, "cached.txt"), []byte("cached prompt"), 0o640); err != nil {
		t.Fatal(err)
	}
	manifest, err := inspectUpgradeLabLegacySessions(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.SessionCount != 1 || manifest.UserMessageCount != 2 || manifest.InteractiveUserMessageCount != 1 || manifest.AssistantMessageCount != 2 {
		t.Fatalf("unexpected session evidence: %#v", manifest)
	}
	if manifest.CatalogSHA256 == "" || manifest.SourceFilesSHA256 == "" || len(manifest.SourceFiles) != 2 {
		t.Fatalf("incomplete session evidence: %#v", manifest)
	}
	if len(manifest.AncillaryFiles) != 1 || manifest.AncillaryFilesSHA256 == "" {
		t.Fatalf("session auxiliary evidence was not separated: %#v", manifest)
	}
}

func TestUpgradeLabConversationCompletedRequiresAssistantAfterMatchingUser(t *testing.T) {
	workspace := t.TempDir()
	sessions := filepath.Join(workspace, "home", ".openclaw", "agents", "main", "sessions")
	if err := os.MkdirAll(sessions, 0o750); err != nil {
		t.Fatal(err)
	}
	marker := "openclaw-upgrade-lab:7:1"
	transcript := strings.Join([]string{
		`{"type":"message","message":{"role":"assistant","content":"earlier"}}`,
		`{"type":"message","message":{"role":"user","content":"` + marker + `"}}`,
	}, "\n") + "\n"
	path := filepath.Join(sessions, "session-1.jsonl")
	if err := os.WriteFile(path, []byte(transcript), 0o640); err != nil {
		t.Fatal(err)
	}
	if completed, err := upgradeLabConversationCompleted(workspace, marker); err != nil || completed {
		t.Fatalf("conversation completed before reply: %v, %v", completed, err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.WriteString(`{"type":"message","message":{"role":"assistant","content":[{"type":"text","text":"done"}]}}` + "\n")
	_ = file.Close()
	if completed, err := upgradeLabConversationCompleted(workspace, marker); err != nil || !completed {
		t.Fatalf("conversation not completed after reply: %v, %v", completed, err)
	}
}

func TestDescribeUpgradeLabRolloutFailureDistinguishesRestoredFromRollbackFailure(t *testing.T) {
	restored := "restored"
	code, phase, message := describeUpgradeLabRolloutFailure(&models.RuntimeRollout{RollbackStatus: &restored}, "migration failed")
	if code != "UPGRADE_FAILED_BASELINE_RESTORED" || phase != "upgrade_failed_restored" || !strings.Contains(message, "已恢复") {
		t.Fatalf("restored failure = %s, %s, %s", code, phase, message)
	}
	rollbackError := "restore failed"
	code, phase, _ = describeUpgradeLabRolloutFailure(&models.RuntimeRollout{RollbackError: &rollbackError}, "migration failed")
	if code != "ROLLBACK_RESTORE_FAILED" || phase != "rollback_failed" {
		t.Fatalf("rollback failure = %s, %s", code, phase)
	}
}

func TestVerifyUpgradeLabSessionArchiveMatchesBytesNotNames(t *testing.T) {
	workspace := t.TempDir()
	archive := filepath.Join(workspace, "home", ".openclaw", "agents", "main", "session-sqlite-import-archive", "run-1")
	if err := os.MkdirAll(archive, 0o750); err != nil {
		t.Fatal(err)
	}
	raw := []byte("legacy transcript\n")
	if err := os.WriteFile(filepath.Join(archive, "renamed.imported-1.jsonl"), raw, 0o640); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	ok, matched, err := verifyUpgradeLabSessionArchive(workspace, map[string]string{"old/name.jsonl": hex.EncodeToString(digest[:])})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || matched != 1 {
		t.Fatalf("archive verification = %v, %d", ok, matched)
	}
}
