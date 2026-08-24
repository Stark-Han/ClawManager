package northbound

import (
	"strings"
	"testing"

	"clawreef/internal/models"
	"clawreef/internal/services"
)

func ownerPointer(value string) *string { return &value }

func TestCoreServiceListsOnlyExactOwnerLiteInstances(t *testing.T) {
	service := &CoreService{instances: &northboundInstanceStub{items: map[int]*models.Instance{
		1: {ID: 1, UserID: 7, Owner: ownerPointer("tenant-a"), Type: services.RuntimeTypeOpenClaw, InstanceMode: services.InstanceModeLite, Name: "lite-match"},
		2: {ID: 2, UserID: 7, Owner: ownerPointer("Tenant-A"), Type: services.RuntimeTypeHermes, InstanceMode: services.InstanceModeLite, Name: "case-mismatch"},
		3: {ID: 3, UserID: 8, Owner: ownerPointer("tenant-a"), Type: services.RuntimeTypeOpenCode, InstanceMode: services.InstanceModeLite, Name: "other-user"},
		4: {ID: 4, UserID: 7, Owner: ownerPointer("tenant-a"), Type: "workbuddy", RuntimeVariant: services.WorkbuddyRuntimeLinux, InstanceMode: services.InstanceModePro, Name: "pro-match"},
		5: {ID: 5, UserID: 7, Owner: ownerPointer("tenant-a"), Type: "custom", InstanceMode: services.InstanceModeLite, Name: "unsupported"},
	}}}

	items, total, err := service.ListInstances(7, " tenant-a ", 1, 20)
	if err != nil {
		t.Fatalf("ListInstances returned error: %v", err)
	}
	if total != 2 || len(items) != 2 {
		t.Fatalf("unexpected owner-scoped instances: total=%d items=%+v", total, items)
	}
	seen := map[int]bool{}
	for _, item := range items {
		seen[item.ID] = true
		if item.Owner != "tenant-a" {
			t.Fatalf("unexpected owner-scoped instance owner: %+v", item)
		}
	}
	if !seen[1] || !seen[4] {
		t.Fatalf("unified list must contain Lite and Linux WorkBuddy Pro: %+v", items)
	}
}

func TestCoreServiceListsOnlyExactOwnerLinuxWorkbuddyProInstances(t *testing.T) {
	service := &CoreService{instances: &northboundInstanceStub{items: map[int]*models.Instance{
		1: {ID: 1, UserID: 7, Owner: ownerPointer("tenant-a"), Type: "workbuddy", RuntimeVariant: "linux", InstanceMode: services.InstanceModePro, Name: "match"},
		2: {ID: 2, UserID: 7, Owner: ownerPointer("tenant-a"), Type: "workbuddy", RuntimeVariant: "windows", InstanceMode: services.InstanceModePro, Name: "windows"},
		3: {ID: 3, UserID: 7, Owner: ownerPointer("tenant-a"), Type: "workbuddy", RuntimeVariant: "linux", InstanceMode: services.InstanceModeLite, Name: "lite"},
		4: {ID: 4, UserID: 8, Owner: ownerPointer("tenant-a"), Type: "workbuddy", RuntimeVariant: "linux", InstanceMode: services.InstanceModePro, Name: "other-user"},
	}}}

	items, total, err := service.ListProInstances(7, " tenant-a ", 1, 20)
	if err != nil {
		t.Fatalf("ListProInstances returned error: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != 1 {
		t.Fatalf("unexpected owner-scoped Pro instances: total=%d items=%+v", total, items)
	}
}

func TestCoreServiceRequiresOwnerForList(t *testing.T) {
	service := &CoreService{instances: &northboundInstanceStub{items: map[int]*models.Instance{}}}
	if _, _, err := service.ListInstances(7, "", 1, 20); apiErrorCode(err) != "INVALID_REQUEST" {
		t.Fatalf("error = %v, want INVALID_REQUEST", err)
	}
}

func TestLiteCreateRequestPropagatesOwner(t *testing.T) {
	request := liteCreateRequest(
		&models.NorthboundOperation{OperationID: "op_owner_test"},
		CreateLiteInstanceRequest{Name: "owner-test", Owner: "tenant-a", Type: services.RuntimeTypeOpenClaw},
	)
	if request.Owner == nil || *request.Owner != "tenant-a" {
		t.Fatalf("owner = %v, want tenant-a", request.Owner)
	}
	if request.ProvisioningOperationID != "op_owner_test" || request.InstanceMode != services.InstanceModeLite {
		t.Fatalf("unexpected create request: %+v", request)
	}
}

func TestNorthboundLiteCreateSupportsEveryManagedLiteRuntime(t *testing.T) {
	for _, instanceType := range []string{
		services.RuntimeTypeOpenClaw,
		services.RuntimeTypeHermes,
		services.RuntimeTypeOpenCode,
		services.RuntimeTypeDeepSeekHarness,
	} {
		if !isSupportedNorthboundType(instanceType) {
			t.Fatalf("runtime %q must be accepted by the northbound Lite contract", instanceType)
		}
		request := liteCreateRequest(
			&models.NorthboundOperation{OperationID: "op_" + instanceType},
			CreateLiteInstanceRequest{Name: instanceType + "-test", Owner: "tenant-a", Type: instanceType},
		)
		if request.Type != instanceType || request.InstanceMode != services.InstanceModeLite || request.RuntimeType != services.RuntimeBackendGateway {
			t.Fatalf("unexpected %s Lite request: %+v", instanceType, request)
		}
	}
	if !isSupportedNorthboundType("workbuddy") {
		t.Fatal("workbuddy must be accepted by the unified northbound contract")
	}
	for _, instanceType := range []string{"codex", "claude-code", "custom"} {
		if isSupportedNorthboundType(instanceType) {
			t.Fatalf("runtime %q must not be accepted by the northbound contract", instanceType)
		}
	}
}

func TestProCreateRequestUsesFixedLinuxWorkbuddyPreset(t *testing.T) {
	t.Setenv("CLAWMANAGER_WORKBUDDY_LINUX_IMAGE", "registry.example/workbuddy-linux:test")
	request := proCreateRequest(
		&models.NorthboundOperation{OperationID: "op_pro_owner_test"},
		CreateProInstanceRequest{Name: "workbuddy-test", Owner: "tenant-a", Type: "workbuddy"},
	)
	if request.Owner == nil || *request.Owner != "tenant-a" {
		t.Fatalf("owner = %v, want tenant-a", request.Owner)
	}
	if request.Type != "workbuddy" || request.RuntimeVariant != services.WorkbuddyRuntimeLinux ||
		request.Mode != services.InstanceModePro || request.InstanceMode != services.InstanceModePro ||
		request.RuntimeType != services.RuntimeBackendDesktop {
		t.Fatalf("unexpected WorkBuddy runtime selection: %+v", request)
	}
	if request.CPUCores != 4 || request.MemoryGB != 8 || request.DiskGB != 40 || request.GPUEnabled || request.GPUCount != 0 {
		t.Fatalf("unexpected WorkBuddy resource preset: %+v", request)
	}
	if request.ImageRegistry == nil || !strings.Contains(*request.ImageRegistry, "workbuddy-linux") {
		t.Fatalf("unexpected WorkBuddy image: %v", request.ImageRegistry)
	}
	if request.ProvisioningOperationID != "op_pro_owner_test" {
		t.Fatalf("operation ID = %q", request.ProvisioningOperationID)
	}
}

func TestOperationCreateRequestSelectsModeFromType(t *testing.T) {
	lite, _, err := operationCreateRequest(&models.NorthboundOperation{
		OperationID:    "op_lite",
		OperationType:  OperationTypeLiteInstance,
		RequestPayload: `{"name":"lite-test","owner":"tenant-a","type":"openclaw"}`,
	})
	if err != nil || lite.InstanceMode != services.InstanceModeLite || lite.Type != "openclaw" {
		t.Fatalf("unexpected Lite operation request: request=%+v err=%v", lite, err)
	}
	workbuddy, _, err := operationCreateRequest(&models.NorthboundOperation{
		OperationID:    "op_unified_workbuddy",
		OperationType:  OperationTypeLiteInstance,
		RequestPayload: `{"name":"workbuddy-test","owner":"tenant-a","type":"workbuddy"}`,
	})
	if err != nil || workbuddy.InstanceMode != services.InstanceModePro || workbuddy.RuntimeVariant != services.WorkbuddyRuntimeLinux {
		t.Fatalf("unexpected unified WorkBuddy operation request: request=%+v err=%v", workbuddy, err)
	}
	// Legacy queued pro_instance operations remain executable during upgrades.
	pro, _, err := operationCreateRequest(&models.NorthboundOperation{
		OperationID:    "op_pro",
		OperationType:  OperationTypeProInstance,
		RequestPayload: `{"name":"pro-test","owner":"tenant-a","type":"workbuddy"}`,
	})
	if err != nil || pro.InstanceMode != services.InstanceModePro || pro.RuntimeVariant != services.WorkbuddyRuntimeLinux {
		t.Fatalf("unexpected Pro operation request: request=%+v err=%v", pro, err)
	}
}
