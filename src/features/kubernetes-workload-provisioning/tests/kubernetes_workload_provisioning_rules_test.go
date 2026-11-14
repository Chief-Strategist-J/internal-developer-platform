package tests

import (
	"testing"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
)

func TestEvaluateWorkloadMutationsDefaults(t *testing.T) {
	cmd := contracts.WorkloadProvisionCommand{
		ID:           "test-1",
		TenantID:     "tenant-alpha",
		ServiceID:    "service-omega",
		Namespace:    "default",
		WorkloadName: "api-workload",
		Image:        "repo/img:1.0.0",
		Replicas:     3,
	}

	mutations := rules.EvaluateWorkloadMutations(cmd)

	if !mutations.SecurityContext.RunAsNonRoot || mutations.SecurityContext.RunAsUser != 10001 {
		t.Errorf("expected non-root UID 10001 security context")
	}
	if !mutations.SecurityContext.ReadOnlyRootFilesystem {
		t.Errorf("expected read-only root filesystem")
	}
	if mutations.CPURequest != "250m" || mutations.CPULimit != "1000m" {
		t.Errorf("expected default CPU 250m/1000m, got %s/%s", mutations.CPURequest, mutations.CPULimit)
	}
	if mutations.MemoryRequest != "256Mi" || mutations.MemoryLimit != "1Gi" {
		t.Errorf("expected default Memory 256Mi/1Gi, got %s/%s", mutations.MemoryRequest, mutations.MemoryLimit)
	}
	if !mutations.PDBRequired || mutations.MinAvailable != 1 {
		t.Errorf("expected PDB with minAvailable=1 for replicas=3")
	}
}

func TestEvaluateWorkloadMutationsSingleReplica(t *testing.T) {
	cmd := contracts.WorkloadProvisionCommand{
		ID:            "test-2",
		TenantID:      "tenant-alpha",
		ServiceID:     "service-omega",
		Namespace:     "default",
		WorkloadName:  "worker-workload",
		Image:         "repo/img:1.0.0",
		CPURequest:    "500m",
		CPULimit:      "2000m",
		MemoryRequest: "512Mi",
		MemoryLimit:   "2Gi",
		Replicas:      1,
	}

	mutations := rules.EvaluateWorkloadMutations(cmd)

	if mutations.PDBRequired {
		t.Errorf("expected PDB not required for replicas=1")
	}
	if mutations.CPURequest != "500m" || mutations.CPULimit != "2000m" {
		t.Errorf("expected custom CPU values preserved")
	}
}

