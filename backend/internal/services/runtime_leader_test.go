package services

import (
	"context"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRuntimeLeaderServiceFirstCallerBecomesLeader(t *testing.T) {
	service := NewRuntimeLeaderService(fake.NewSimpleClientset(), "runtime-system", "backend-a")

	if !service.IsLeader(context.Background()) {
		t.Fatalf("expected first caller to become leader")
	}
}

func TestRuntimeLeaderServiceDifferentCallerCannotStealUnexpiredLease(t *testing.T) {
	now := metav1.NewMicroTime(time.Now())
	holder := "backend-a"
	client := fake.NewSimpleClientset(&coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: "clawmanager-runtime-scheduler", Namespace: "runtime-system"},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &holder,
			RenewTime:            &now,
			LeaseDurationSeconds: int32Ptr(15),
		},
	})
	service := NewRuntimeLeaderService(client, "runtime-system", "backend-b")

	if service.IsLeader(context.Background()) {
		t.Fatalf("expected different caller not to steal unexpired lease")
	}
}

func TestRuntimeLeaderServiceDifferentCallerCanAcquireExpiredLease(t *testing.T) {
	renewTime := metav1.NewMicroTime(time.Now().Add(-30 * time.Second))
	holder := "backend-a"
	client := fake.NewSimpleClientset(&coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: "clawmanager-runtime-scheduler", Namespace: "runtime-system"},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       &holder,
			RenewTime:            &renewTime,
			LeaseDurationSeconds: int32Ptr(15),
		},
	})
	service := NewRuntimeLeaderService(client, "runtime-system", "backend-b")

	if !service.IsLeader(context.Background()) {
		t.Fatalf("expected different caller to acquire expired lease")
	}

	lease, err := client.CoordinationV1().Leases("runtime-system").Get(context.Background(), "clawmanager-runtime-scheduler", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to load lease: %v", err)
	}
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != "backend-b" {
		t.Fatalf("expected lease holder backend-b, got %#v", lease.Spec.HolderIdentity)
	}
}

func int32Ptr(value int32) *int32 {
	return &value
}
