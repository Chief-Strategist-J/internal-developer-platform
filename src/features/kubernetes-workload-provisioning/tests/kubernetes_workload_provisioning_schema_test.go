// Package tests provides unit tests for Kubernetes Workload Provisioning contracts and schemas.
//
// Architectural Blueprint:
// - Step 14 Schema Test: Validates schema parsing, bounds enforcement, and command validation.
// - 100% comment-free function bodies.
package tests

import (
	"testing"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
)

func TestValidateWorkloadCommand(t *testing.T) {
	valid := contracts.WorkloadProvisionCommand{
		ID:             "workload-101",
		TenantID:       "tenant-dev",
		ServiceID:      "billing-api",
		Namespace:      "billing-dev",
		WorkloadName:   "billing-deploy",
		Image:          "ghcr.io/org/billing:v2.1.0",
		CPURequest:     "100m",
		CPULimit:       "500m",
		MemoryRequest:  "128Mi",
		MemoryLimit:    "512Mi",
		Replicas:       2,
		IdempotencyKey: "idemp-wkld-101",
	}

	if err := contracts.ValidateWorkloadCommand(valid); err != nil {
		t.Fatalf("expected valid command to pass, got: %v", err)
	}

	invalidNamespace := valid
	invalidNamespace.Namespace = "Invalid_Namespace"
	if err := contracts.ValidateWorkloadCommand(invalidNamespace); err == nil {
		t.Errorf("expected error on non-RFC-1123 namespace")
	}

	invalidCPU := valid
	invalidCPU.CPURequest = "invalid-cpu"
	if err := contracts.ValidateWorkloadCommand(invalidCPU); err == nil {
		t.Errorf("expected error on invalid CPU quantity")
	}

	invalidReplicas := valid
	invalidReplicas.Replicas = 0
	if err := contracts.ValidateWorkloadCommand(invalidReplicas); err == nil {
		t.Errorf("expected error on replicas < 1")
	}
}
