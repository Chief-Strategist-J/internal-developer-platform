/*
ALGORITHM & ARCHITECTURE BLUEPRINT: POLICY SERVICE & WORKFLOW INTEGRATION TESTS

1. Test Execution Architecture:
   - Step 14 Tests: Validates integration between PolicyAsCodeService, EvaluationWorkflow, and MemoryRepository.
   - Tests lifecycle transitions, exception grants, and admission verdicts.
   - 100% comment-free function bodies.
*/
package tests

import (
	"context"
	"testing"
	"time"

	"internal-developer-platform/src/features/policy-as-code/contracts"
	"internal-developer-platform/src/features/policy-as-code/domain/machines"
	"internal-developer-platform/src/features/policy-as-code/domain/workflows"
	"internal-developer-platform/src/features/policy-as-code/infrastructure/adapters"
	"internal-developer-platform/src/features/policy-as-code/service"
)

func TestPolicyAsCodeService_CompleteLifecycleAndAdmission(t *testing.T) {
	ctx := context.Background()
	repo := adapters.NewMemoryPolicyRepositoryAdapter()
	svc := service.NewPolicyAsCodeService(repo)

	tenantID := "tenant-ecommerce"

	policyInput := contracts.PolicyRecord{
		TenantID:         tenantID,
		Name:             "Read Only Root Filesystem Enforcer",
		Description:      "Containers must use read-only root filesystems",
		ExternalID:       "POD-11",
		EnforcementPoint: contracts.EnforcementPointCEL,
		Severity:         contracts.SeverityHigh,
		RuleExpression:   "readOnlyRootFilesystem == true",
		Exceptable:       true,
		DetectionPair:    "pc-audit-pod11",
		Source:           contracts.PolicySourceAPI,
	}

	created, err := svc.CreatePolicy(ctx, policyInput)
	if err != nil {
		t.Fatalf("failed to create policy: %v", err)
	}
	if created.LifecycleState != contracts.LifecycleDraft {
		t.Fatalf("expected initial state DRAFT, got: %s", created.LifecycleState)
	}

	stages := []machines.PolicyLifecycleEvent{
		machines.EventPromoteToCIOnly,
		machines.EventPromoteToAudit,
		machines.EventPromoteToWarn,
		machines.EventPromoteToDeny,
	}

	for _, st := range stages {
		created, err = svc.TransitionPolicyLifecycle(ctx, tenantID, created.ID, st, 0)
		if err != nil {
			t.Fatalf("failed transition via %s: %v", st, err)
		}
	}

	if created.LifecycleState != contracts.LifecycleDeny {
		t.Fatalf("expected state DENY, got: %s", created.LifecycleState)
	}

	workloadID := "wkld-cart-service"
	namespace := "ecommerce-prod"
	badManifest := `{"readOnlyRootFilesystem":false}`

	evalInput := workflows.AdmissionEvaluationInput{
		TenantID:     tenantID,
		Namespace:    namespace,
		WorkloadID:   workloadID,
		CallerRole:   "Developer",
		ResourceJSON: badManifest,
		Policies:     []contracts.PolicyRecord{*created},
		Timestamp:    time.Now().UTC(),
	}

	verdict, err := svc.EvaluateAdmission(ctx, evalInput)
	if err != nil {
		t.Fatalf("evaluation returned unexpected error: %v", err)
	}
	if verdict.Allowed {
		t.Fatalf("expected admission to be REJECTED for non-compliant manifest")
	}

	excReq := contracts.PolicyExceptionRecord{
		TenantID:      tenantID,
		PolicyID:      created.ID,
		WorkloadID:    workloadID,
		Namespace:     namespace,
		ApproverID:    "security-officer-01",
		Justification: "Cart service requires temporary local buffer write",
		ExpiresAt:     time.Now().UTC().Add(14 * 24 * time.Hour),
	}

	_, err = svc.RequestException(ctx, excReq)
	if err != nil {
		t.Fatalf("failed to grant valid exception: %v", err)
	}

	verdictAfterException, err := svc.EvaluateAdmission(ctx, evalInput)
	if err != nil {
		t.Fatalf("evaluation after exception returned error: %v", err)
	}
	if !verdictAfterException.Allowed {
		t.Fatalf("expected admission to be ALLOWED due to active exception, denial: %s", verdictAfterException.DenialReason)
	}
	if len(verdictAfterException.AppliedExceptions) == 0 || verdictAfterException.AppliedExceptions[0] != "POD-11" {
		t.Fatalf("expected applied exception POD-11, got: %v", verdictAfterException.AppliedExceptions)
	}
}

