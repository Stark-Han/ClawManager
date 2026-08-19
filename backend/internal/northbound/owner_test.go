package northbound

import (
	"testing"

	"clawreef/internal/models"
	"clawreef/internal/services"
)

func ownerPointer(value string) *string { return &value }

func TestCoreServiceListsOnlyExactOwnerLiteInstances(t *testing.T) {
	service := &CoreService{instances: &northboundInstanceStub{items: map[int]*models.Instance{
		1: {ID: 1, UserID: 7, Owner: ownerPointer("tenant-a"), InstanceMode: services.InstanceModeLite, Name: "match"},
		2: {ID: 2, UserID: 7, Owner: ownerPointer("Tenant-A"), InstanceMode: services.InstanceModeLite, Name: "case-mismatch"},
		3: {ID: 3, UserID: 8, Owner: ownerPointer("tenant-a"), InstanceMode: services.InstanceModeLite, Name: "other-user"},
		4: {ID: 4, UserID: 7, Owner: ownerPointer("tenant-a"), InstanceMode: services.InstanceModePro, Name: "pro"},
	}}}

	items, total, err := service.ListInstances(7, " tenant-a ", 1, 20)
	if err != nil {
		t.Fatalf("ListInstances returned error: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != 1 || items[0].Owner != "tenant-a" {
		t.Fatalf("unexpected owner-scoped instances: total=%d items=%+v", total, items)
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
