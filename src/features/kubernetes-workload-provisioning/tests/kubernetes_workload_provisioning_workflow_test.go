// Package tests provides unit tests for Kubernetes Workload Provisioning workflows.
//
// Architectural Blueprint:
// - Step 14 Workflow Test: Exhaustive unit tests for domain automation DAG steps.
// - Tests both Provisioning DAG and Decommissioning DAG execution pipelines.
// - 100% comment-free function bodies.
package tests

import (
	"context"
	"strings"
	"testing"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/workflows"
)

func TestExecuteProvisionDAG_SynthesizesCanaryAndCloudBundle(t *testing.T) {
	ctx := context.Background()
	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-canary-dag-01",
		TenantID:       "tenant-dag",
		ServiceID:      "dag-api",
		Namespace:      "dag-prod",
		WorkloadName:   "dag-worker",
		Image:          "gcr.io/bank/dag:v1.0.0",
		CPURequest:     "500m",
		CPULimit:       "1000m",
		MemoryRequest:  "1Gi",
		MemoryLimit:    "2Gi",
		Replicas:       3,
		IdempotencyKey: "idemp-dag-01",
	}

	mutations := rules.EvaluateWorkloadMutations(cmd)
	exec, bundle, err := workflows.ExecuteProvisionDAG(ctx, cmd, mutations)
	if err != nil {
		t.Fatalf("unexpected error executing DAG: %v", err)
	}

	if len(exec.Steps) != 9 {
		t.Errorf("expected 9 DAG steps, got %d", len(exec.Steps))
	}

	if bundle == nil {
		t.Fatalf("expected non-nil synthesized canary bundle")
	}

	if !strings.Contains(bundle.RolloutYAML, "kind: Rollout") {
		t.Errorf("expected kind: Rollout in synthesized bundle")
	}
	if !strings.Contains(bundle.VirtualServiceYAML, "kind: VirtualService") {
		t.Errorf("expected kind: VirtualService in synthesized bundle")
	}
	if !strings.Contains(bundle.DestinationRuleYAML, "kind: DestinationRule") {
		t.Errorf("expected kind: DestinationRule in synthesized bundle")
	}
	if !strings.Contains(bundle.WorkloadIdentityYAML, "kind: IAMPolicyMember") {
		t.Errorf("expected kind: IAMPolicyMember in synthesized bundle")
	}
	if !strings.Contains(bundle.DatabaseYAML, "kind: SQLDatabase") {
		t.Errorf("expected kind: SQLDatabase in synthesized bundle")
	}
}

func TestExecuteDecommissionDAG_ExecutesAllSteps(t *testing.T) {
	ctx := context.Background()
	cmd := contracts.WorkloadDecommissionCommand{
		TenantID:     "tenant-decom-01",
		Namespace:    "payments-prod",
		WorkloadName: "worker-service",
		Decision: contracts.DecommissionDecision{
			ApprovedBy:       "sre-lead",
			Reason:           "service retirement",
			RetentionDays:    30,
			LockBackups:      true,
			DecisionRecorded: true,
		},
	}

	steps, err := workflows.ExecuteDecommissionDAG(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error executing decommission DAG: %v", err)
	}

	if len(steps) != 7 {
		t.Fatalf("expected 7 decommission steps, got %d", len(steps))
	}

	expectedSteps := []contracts.DecommissionExecutionStep{
		contracts.StepDecommissionDrainTraffic,
		contracts.StepDecommissionRemoveWorkload,
		contracts.StepDecommissionRemoveBindings,
		contracts.StepDecommissionReleaseStorage,
		contracts.StepDecommissionDeprovisionIdentity,
		contracts.StepDecommissionRemoveDNS,
		contracts.StepDecommissionRecordAudit,
	}

	for i, expected := range expectedSteps {
		if steps[i] != expected {
			t.Errorf("step %d mismatch: expected %s, got %s", i, expected, steps[i])
		}
	}
}

func TestExecuteProvisionDAG_InvalidSpecRejection(t *testing.T) {
	ctx := context.Background()
	invalidCmd := contracts.WorkloadProvisionCommand{
		ID:             "",
		TenantID:       "",
		ServiceID:      "",
		Namespace:      "INVALID_NAMESPACE",
		WorkloadName:   "",
		Image:          "",
		Replicas:       0,
		IdempotencyKey: "",
	}

	mutations := rules.EvaluateWorkloadMutations(invalidCmd)
	_, _, err := workflows.ExecuteProvisionDAG(ctx, invalidCmd, mutations)
	if err == nil {
		t.Fatalf("expected error executing provision DAG with invalid command")
	}
}
