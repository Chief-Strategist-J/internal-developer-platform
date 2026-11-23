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

func TestPolicyViolationPersistenceAndFSMGuard(t *testing.T) {
	ctx := context.Background()
	repo := adapters.NewMemoryPolicyRepositoryAdapter()
	svc := service.NewPolicyAsCodeService(repo)

	tenantID := "tenant-fsm-test"
	p := contracts.PolicyRecord{
		ID:               "pol-fsm-guard-01",
		PublicID:         "pol_guard_01",
		TenantID:         tenantID,
		ExternalID:       "POD-01",
		Name:             "No Privileged Containers",
		Description:      "Containers must not run in privileged mode",
		EnforcementPoint: contracts.EnforcementPointCEL,
		Severity:         contracts.SeverityCritical,
		RuleExpression:   "privileged == false",
		Exceptable:       false,
		DetectionPair:    "PC audit",
		LifecycleState:   contracts.LifecycleDraft,
		Source:           contracts.PolicySourceAPI,
	}

	created, err := svc.CreatePolicy(ctx, p)
	if err != nil {
		t.Fatalf("failed to create policy: %v", err)
	}

	pol, err := svc.TransitionPolicyLifecycle(ctx, tenantID, created.ID, machines.EventPromoteToCIOnly, 0)
	if err != nil {
		t.Fatalf("failed transition to CI_ONLY: %v", err)
	}

	pol, err = svc.TransitionPolicyLifecycle(ctx, tenantID, pol.ID, machines.EventPromoteToAudit, 0)
	if err != nil {
		t.Fatalf("failed transition to AUDIT: %v", err)
	}

	pol, err = svc.TransitionPolicyLifecycle(ctx, tenantID, pol.ID, machines.EventPromoteToWarn, 0)
	if err != nil {
		t.Fatalf("failed transition to WARN: %v", err)
	}

	evalInput := workflows.AdmissionEvaluationInput{
		TenantID:     tenantID,
		Namespace:    "orders-prod",
		WorkloadID:   "orders-worker",
		CallerRole:   "Developer",
		ResourceJSON: `{"privileged":true}`,
		Policies:     []contracts.PolicyRecord{*pol},
		Timestamp:    time.Now().UTC(),
	}

	verdict, err := svc.EvaluateAdmission(ctx, evalInput)
	if err != nil {
		t.Fatalf("evaluation returned unexpected error: %v", err)
	}
	if len(verdict.Warnings) == 0 {
		t.Fatalf("expected warning for WARN policy violation")
	}

	vCount, err := svc.CountActiveViolations(ctx, tenantID, pol.ID)
	if err != nil {
		t.Fatalf("failed to count violations: %v", err)
	}
	if vCount != 1 {
		t.Fatalf("expected 1 active violation in db, got: %d", vCount)
	}

	_, err = svc.TransitionPolicyLifecycle(ctx, tenantID, pol.ID, machines.EventPromoteToDeny, 0)
	if err == nil {
		t.Fatalf("expected promotion to DENY to fail when active violations exist in db")
	}

	viols, err := svc.ListActiveViolations(ctx, tenantID, pol.ID)
	if err != nil {
		t.Fatalf("failed to list active violations: %v", err)
	}
	if len(viols) != 1 {
		t.Fatalf("expected 1 violation record, got: %d", len(viols))
	}

	err = svc.ResolveViolation(ctx, tenantID, viols[0].ID, viols[0].Version)
	if err != nil {
		t.Fatalf("failed to resolve violation: %v", err)
	}

	pol, err = svc.TransitionPolicyLifecycle(ctx, tenantID, pol.ID, machines.EventPromoteToDeny, 0)
	if err != nil {
		t.Fatalf("expected promotion to DENY to succeed after resolution, got: %v", err)
	}
	if pol.LifecycleState != contracts.LifecycleDeny {
		t.Fatalf("expected state DENY, got: %s", pol.LifecycleState)
	}
}

func TestContinuousAuditScanWorkflow(t *testing.T) {
	ctx := context.Background()
	repo := adapters.NewMemoryPolicyRepositoryAdapter()
	svc := service.NewPolicyAsCodeService(repo)

	tenantID := "tenant-audit-test"
	pol := contracts.PolicyRecord{
		ID:               "pol-audit-scan-01",
		PublicID:         "pol_as_01",
		TenantID:         tenantID,
		ExternalID:       "SEC-AUDIT-01",
		Name:             "Audit Scan Non-root Rule",
		Description:      "Enforce non-root execution",
		EnforcementPoint: contracts.EnforcementPointCEL,
		Severity:         contracts.SeverityCritical,
		RuleExpression:   "runAsNonRoot == true",
		Exceptable:       true,
		DetectionPair:    "DET-AUDIT-01",
		LifecycleState:   contracts.LifecycleDeny,
		Source:           contracts.PolicySourceAPI,
	}

	_, err := svc.CreatePolicy(ctx, pol)
	if err != nil {
		t.Fatalf("failed to create policy: %v", err)
	}

	input := workflows.AuditScanInput{
		TenantID: tenantID,
		Workloads: []workflows.WorkloadManifest{
			{
				Namespace:    "finance-prod",
				WorkloadID:   "w-ledger-api",
				ResourceJSON: `{"runAsNonRoot":false}`,
			},
			{
				Namespace:    "finance-prod",
				WorkloadID:   "w-compliant-worker",
				ResourceJSON: `{"runAsNonRoot":true}`,
			},
		},
		Policies:  []contracts.PolicyRecord{pol},
		Timestamp: time.Now().UTC(),
	}

	report, err := svc.ExecuteAuditScan(ctx, input)
	if err != nil {
		t.Fatalf("audit scan returned error: %v", err)
	}

	if report.TotalWorkloadsScanned != 2 {
		t.Fatalf("expected 2 workloads scanned, got: %d", report.TotalWorkloadsScanned)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected 1 finding, got: %d", len(report.Findings))
	}
	if report.Findings[0].WorkloadID != "w-ledger-api" {
		t.Fatalf("expected finding on w-ledger-api, got: %s", report.Findings[0].WorkloadID)
	}

	activeCount, err := svc.CountActiveViolations(ctx, tenantID, pol.ID)
	if err != nil {
		t.Fatalf("failed to count violations: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("expected 1 active violation in db after scan, got: %d", activeCount)
	}
}

func TestExceptionReconcileExpiredExceptions_Integration(t *testing.T) {
	ctx := context.Background()
	repo := adapters.NewMemoryPolicyRepositoryAdapter()
	svc := service.NewPolicyAsCodeService(repo)

	tenantID := "tenant-reconcile-test"
	now := time.Now().UTC()

	pol := contracts.PolicyRecord{
		ID:               "pol-reconcile-01",
		PublicID:         "pol_rec_01",
		TenantID:         tenantID,
		ExternalID:       "POD-11",
		Name:             "Read Only Root Filesystem Enforcer",
		Description:      "Containers must use read-only root filesystems",
		EnforcementPoint: contracts.EnforcementPointCEL,
		Severity:         contracts.SeverityCritical,
		RuleExpression:   "readOnlyRootFilesystem == true",
		Exceptable:       true,
		DetectionPair:    "PC audit",
		LifecycleState:   contracts.LifecycleDeny,
		Source:           contracts.PolicySourceAPI,
		Version:          1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	_, err := svc.CreatePolicy(ctx, pol)
	if err != nil {
		t.Fatalf("failed to create policy: %v", err)
	}

	excReq := contracts.PolicyExceptionRecord{
		ID:            "exc-reconcile-01",
		PublicID:      "exc_rec_01",
		TenantID:      tenantID,
		PolicyID:      pol.ID,
		WorkloadID:    "w-expired-pod",
		Namespace:     "prod-reconcile",
		ApproverID:    "sec-lead",
		Justification: "Temporary bypass that has expired",
		Status:        contracts.ExceptionStatusActive,
		ExpiresAt:     now.Add(-10 * time.Minute),
		CorrelationID: "corr-1",
		Version:       1,
		CreatedAt:     now.Add(-20 * time.Minute),
		UpdatedAt:     now.Add(-20 * time.Minute),
	}

	if err := repo.SaveException(ctx, excReq); err != nil {
		t.Fatalf("failed to insert expired exception: %v", err)
	}

	reconciled, err := svc.ReconcileExpiredExceptions(ctx, tenantID)
	if err != nil {
		t.Fatalf("failed to reconcile expired exceptions: %v", err)
	}
	if reconciled != 1 {
		t.Fatalf("expected 1 exception reconciled, got: %d", reconciled)
	}

	evalInput := workflows.AdmissionEvaluationInput{
		TenantID:     tenantID,
		Namespace:    "prod-reconcile",
		WorkloadID:   "w-expired-pod",
		CallerRole:   "Developer",
		ResourceJSON: `{"readOnlyRootFilesystem":false}`,
		Timestamp:    now,
	}

	verdict, err := svc.EvaluateAdmission(ctx, evalInput)
	if err != nil {
		t.Fatalf("evaluation returned unexpected error: %v", err)
	}
	if verdict.Allowed {
		t.Fatalf("expected admission to be REJECTED because exception has expired")
	}
}

func TestPolicyAsCodeService_AdmissionEvaluation_AdHocPolicy_SkipsUnregisteredViolationPersistence(t *testing.T) {
	ctx := context.Background()
	repo := adapters.NewMemoryPolicyRepositoryAdapter()
	svc := service.NewPolicyAsCodeService(repo)

	tenantID := "tenant-adhoc"
	now := time.Now().UTC()

	adHocPolicy := contracts.PolicyRecord{
		ID:               "adhoc-pol-01",
		TenantID:         tenantID,
		Name:             "AdHoc Disallow Privileged",
		Description:      "Adhoc rule not persisted to database",
		ExternalID:       "SEC-ADHOC-01",
		EnforcementPoint: contracts.EnforcementPointCEL,
		Severity:         contracts.SeverityCritical,
		RuleExpression:   "privileged == false",
		Exceptable:       false,
		DetectionPair:    "det-adhoc-01",
		LifecycleState:   contracts.LifecycleDeny,
		Source:           contracts.PolicySourceAPI,
	}

	evalInput := workflows.AdmissionEvaluationInput{
		TenantID:     tenantID,
		Namespace:    "fintech-prod",
		WorkloadID:   "ledger-worker",
		CallerRole:   "Developer",
		ResourceJSON: `{"privileged":true}`,
		Policies:     []contracts.PolicyRecord{adHocPolicy},
		Timestamp:    now,
	}

	verdict, err := svc.EvaluateAdmission(ctx, evalInput)
	if err != nil {
		t.Fatalf("unexpected error evaluating ad-hoc policy: %v", err)
	}
	if verdict.Allowed {
		t.Fatalf("expected admission to be REJECTED for privileged container")
	}
	if len(verdict.Violations) != 1 {
		t.Fatalf("expected 1 violation in verdict, got: %d", len(verdict.Violations))
	}

	viols, err := repo.ListActiveViolationsByPolicy(ctx, tenantID, adHocPolicy.ID)
	if err != nil {
		t.Fatalf("unexpected error querying violations: %v", err)
	}
	if len(viols) != 0 {
		t.Fatalf("expected 0 violations persisted for unregistered policy, got: %d", len(viols))
	}
}

