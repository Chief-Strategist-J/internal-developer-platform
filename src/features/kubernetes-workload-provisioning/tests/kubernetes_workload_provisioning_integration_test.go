package tests

import (
	"context"
	"testing"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	kubernetesworkloadprovisioning "internal-developer-platform/src/features/kubernetes-workload-provisioning"
)

func TestWorkloadProvisioningServiceExecution(t *testing.T) {
	ctx := context.Background()
	store := kubernetesworkloadprovisioning.NewMemoryStore()
	k8sClient := kubernetesworkloadprovisioning.NewK8sClient("https://k8s-api.internal:6443")
	svc := kubernetesworkloadprovisioning.NewService(store, k8sClient)

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-test-1",
		TenantID:       "tenant-qa",
		ServiceID:      "auth-service",
		Namespace:      "auth-qa",
		WorkloadName:   "auth-deploy",
		Image:          "repo/auth:latest",
		CPURequest:     "",
		CPULimit:       "",
		MemoryRequest:  "",
		MemoryLimit:    "",
		Replicas:       3,
		IdempotencyKey: "idemp-wkld-001",
	}

	result, err := svc.ProvisionWorkload(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error provisioning workload: %v", err)
	}

	if result.Provision.Status != contracts.StatusActive {
		t.Errorf("expected status ACTIVE, got %s", result.Provision.Status)
	}
	if result.Provision.CPURequest != "250m" {
		t.Errorf("expected default injected CPURequest 250m, got %s", result.Provision.CPURequest)
	}
	if !result.PDBCreated {
		t.Errorf("expected PDB created for replicas=3")
	}

	replay, err := svc.ProvisionWorkload(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error on idempotent replay: %v", err)
	}
	if replay.Message != "idempotent replay" {
		t.Errorf("expected idempotent replay message, got: %s", replay.Message)
	}

	fetched, err := svc.GetWorkload(ctx, "tenant-qa", "wkld-test-1")
	if err != nil {
		t.Fatalf("unexpected error fetching record: %v", err)
	}
	if fetched.ID != "wkld-test-1" {
		t.Errorf("expected ID wkld-test-1, got %s", fetched.ID)
	}

	list, err := svc.ListWorkloads(ctx, contracts.FilterCriteria{TenantID: "tenant-qa"})
	if err != nil {
		t.Fatalf("unexpected error listing records: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 record in list, got %d", len(list))
	}
}

