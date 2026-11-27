/*
ALGORITHM & ARCHITECTURE BLUEPRINT: POLICY-AS-CODE RULES & STATE MACHINE TESTS

1. Test Execution Architecture:
  - Step 14 Tests: Unit test suite for domain machines and domain rules.
  - Hermetic, in-memory, deterministic execution with no network/database dependencies.
  - 100% comment-free function bodies.
*/
package tests

import (
	"testing"
	"time"

	"internal-developer-platform/src/features/policy-as-code/internal/contracts"
	"internal-developer-platform/src/features/policy-as-code/internal/domain/machines"
	"internal-developer-platform/src/features/policy-as-code/internal/domain/rules"
)

func TestPolicyLifecycleStateMachine_ValidPromotionRing(t *testing.T) {
	state := contracts.LifecycleDraft

	next, err := machines.TransitionPolicyLifecycle(state, machines.EventPromoteToCIOnly, 0)
	if err != nil || next != contracts.LifecycleCIOnly {
		t.Fatalf("expected CI_ONLY, got %s, err: %v", next, err)
	}

	next, err = machines.TransitionPolicyLifecycle(next, machines.EventPromoteToAudit, 0)
	if err != nil || next != contracts.LifecycleAudit {
		t.Fatalf("expected AUDIT, got %s, err: %v", next, err)
	}

	next, err = machines.TransitionPolicyLifecycle(next, machines.EventPromoteToWarn, 0)
	if err != nil || next != contracts.LifecycleWarn {
		t.Fatalf("expected WARN, got %s, err: %v", next, err)
	}

	next, err = machines.TransitionPolicyLifecycle(next, machines.EventPromoteToDeny, 0)
	if err != nil || next != contracts.LifecycleDeny {
		t.Fatalf("expected DENY, got %s, err: %v", next, err)
	}

	next, err = machines.TransitionPolicyLifecycle(next, machines.EventRetire, 0)
	if err != nil || next != contracts.LifecycleRetired {
		t.Fatalf("expected RETIRED, got %s, err: %v", next, err)
	}
}

func TestPolicyLifecycleStateMachine_WarnToDenyViolationGuard(t *testing.T) {
	state := contracts.LifecycleWarn

	_, err := machines.TransitionPolicyLifecycle(state, machines.EventPromoteToDeny, 5)
	if err == nil {
		t.Fatalf("expected conflict error when promoting to DENY with active violations > 0")
	}

	next, err := machines.TransitionPolicyLifecycle(state, machines.EventPromoteToDeny, 0)
	if err != nil || next != contracts.LifecycleDeny {
		t.Fatalf("expected successful promotion to DENY with 0 violations, got %s, err: %v", next, err)
	}
}

func TestPolicyLifecycleStateMachine_RollbackAndReopen(t *testing.T) {
	statesToRollback := []contracts.PolicyLifecycleState{
		contracts.LifecycleDraft,
		contracts.LifecycleCIOnly,
		contracts.LifecycleAudit,
		contracts.LifecycleWarn,
		contracts.LifecycleDeny,
	}

	for _, s := range statesToRollback {
		rolledBack, err := machines.TransitionPolicyLifecycle(s, machines.EventRollback, 0)
		if err != nil || rolledBack != contracts.LifecycleRolledBack {
			t.Fatalf("failed rollback from %s: %v", s, err)
		}
	}

	reopened, err := machines.TransitionPolicyLifecycle(contracts.LifecycleRolledBack, machines.EventReopenDraft, 0)
	if err != nil || reopened != contracts.LifecycleDraft {
		t.Fatalf("failed to reopen draft from rolled back: %v", err)
	}

	_, err = machines.TransitionPolicyLifecycle(contracts.LifecycleRetired, machines.EventRollback, 0)
	if err == nil {
		t.Fatalf("expected error attempting to roll back retired policy")
	}
}

func TestPolicyLifecycleStateMachine_IllegalTransitions(t *testing.T) {
	_, err := machines.TransitionPolicyLifecycle(contracts.LifecycleDraft, machines.EventPromoteToDeny, 0)
	if err == nil {
		t.Fatalf("expected error on skip transition from DRAFT to DENY")
	}

	_, err = machines.TransitionPolicyLifecycle(contracts.LifecycleRetired, machines.EventPromoteToCIOnly, 0)
	if err == nil {
		t.Fatalf("expected error on transition from terminal RETIRED state")
	}
}

func TestPolicyRules_NonExceptableCatalog(t *testing.T) {
	blockedRules := []string{
		"POD-01", "POD-05", "POD-09",
		"IMG-01", "IMG-02", "IMG-03",
		"RBAC-01", "RBAC-04",
		"NET-02", "NET-03",
		"TEN-02", "TEN-06",
		"CLD-01", "CLD-04", "CLD-06",
	}

	for _, r := range blockedRules {
		if rules.IsRuleExceptable(r) {
			t.Errorf("expected rule %s to be non-exceptable", r)
		}
	}

	allowedRules := []string{
		"POD-11", "IMG-04", "SA-02", "NET-04", "GIT-04", "SEC-03", "STO-03", "AVL-02",
	}

	for _, r := range allowedRules {
		if !rules.IsRuleExceptable(r) {
			t.Errorf("expected rule %s to be exceptable", r)
		}
	}
}

func TestPolicyRules_ValidateExceptionEligibility(t *testing.T) {
	now := time.Now().UTC()

	nonExceptablePolicy := contracts.PolicyRecord{
		ExternalID: "POD-01",
		Name:       "No Privileged Containers",
		Exceptable: false,
	}

	validExceptionReq := contracts.PolicyExceptionRecord{
		ApproverID: "security-lead",
		TenantID:   "fintech-team",
		ExpiresAt:  now.Add(30 * 24 * time.Hour),
	}

	err := rules.ValidateExceptionEligibility(nonExceptablePolicy, validExceptionReq, now)
	if err == nil {
		t.Fatalf("expected error for non-exceptable policy exception")
	}

	exceptablePolicy := contracts.PolicyRecord{
		ExternalID: "POD-11",
		Name:       "Read Only Root Filesystem",
		Exceptable: true,
	}

	err = rules.ValidateExceptionEligibility(exceptablePolicy, validExceptionReq, now)
	if err != nil {
		t.Fatalf("expected valid exception to pass, got: %v", err)
	}

	conflictReq := validExceptionReq
	conflictReq.ApproverID = "fintech-team"
	err = rules.ValidateExceptionEligibility(exceptablePolicy, conflictReq, now)
	if err == nil {
		t.Fatalf("expected error on separation of duties violation")
	}

	expiredReq := validExceptionReq
	expiredReq.ExpiresAt = now.Add(-1 * time.Hour)
	err = rules.ValidateExceptionEligibility(exceptablePolicy, expiredReq, now)
	if err == nil {
		t.Fatalf("expected error on past expiration date")
	}

	excessiveReq := validExceptionReq
	excessiveReq.ExpiresAt = now.Add(120 * 24 * time.Hour)
	err = rules.ValidateExceptionEligibility(exceptablePolicy, excessiveReq, now)
	if err == nil {
		t.Fatalf("expected error on lease duration > 90 days")
	}
}

func TestPolicyRules_ValidateCELRuleExpression(t *testing.T) {
	validExpressions := []string{
		"request.auth.claims.role == 'Admin'",
		"has(object.spec.securityContext) && object.spec.securityContext.runAsNonRoot == true",
		"[1, 2, 3].all(x, x > 0)",
		"{'env': 'prod', 'tier': '0'}.size() > 0",
	}

	for _, expr := range validExpressions {
		if err := rules.ValidateCELRuleExpression(expr); err != nil {
			t.Errorf("expected valid expression '%s' to pass, got: %v", expr, err)
		}
	}

	invalidExpressions := []string{
		"",
		"   ",
		"request.auth.claims == 'Admin')",
		"(has(object.spec.securityContext)",
		"[1, 2, 3].all(x, x > 0",
		"{'env': 'prod'",
	}

	for _, expr := range invalidExpressions {
		if err := rules.ValidateCELRuleExpression(expr); err == nil {
			t.Errorf("expected invalid expression '%s' to fail", expr)
		}
	}
}
