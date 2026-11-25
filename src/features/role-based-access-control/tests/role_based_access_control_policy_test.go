package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/features/role-based-access-control/domain/rules"
	"internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
	"internal-developer-platform/src/features/role-based-access-control/service"
)

type mockRBACOutbox struct {
	events []rbacContracts.RBACAuditEvent
	fail   bool
}

func (m *mockRBACOutbox) EnqueueAuditEvent(ctx context.Context, ev rbacContracts.RBACAuditEvent) error {
	if m.fail {
		return errors.New("audit outbox unavailable")
	}
	m.events = append(m.events, ev)
	return nil
}

func TestRBACExplicitDenyAndRootGuardrails(t *testing.T) {
	now := time.Now()
	devWithDeny := rbacContracts.Subject{
		ID:       "dev-1",
		TenantID: "t1",
		Roles:    []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
		DenyPermissions: []rbacContracts.Permission{
			{Resource: rbacContracts.ResourceServiceCatalog, Action: rbacContracts.ActionWrite},
		},
	}
	res := rules.EvaluateAuthorization(rbacContracts.EvaluationRequest{
		Subject:        devWithDeny,
		TargetTenantID: "t1",
		Resource:       rbacContracts.ResourceServiceCatalog,
		Action:         rbacContracts.ActionWrite,
	}, now)
	if res.Granted || res.ViolationCode != "ERR_EXPLICIT_DENY" {
		t.Fatalf("expected ERR_EXPLICIT_DENY, got %+v", res)
	}

	admin := rbacContracts.Subject{
		ID:    "admin-1",
		Roles: []rbacContracts.RoleName{rbacContracts.RolePlatformAdmin},
	}
	guardRes := rules.EvaluateAuthorization(rbacContracts.EvaluationRequest{
		Subject:  admin,
		Resource: rbacContracts.ResourceAuditLogs,
		Action:   rbacContracts.ActionDelete,
	}, now)
	if guardRes.Granted || guardRes.ViolationCode != "ERR_ROOT_GUARDRAIL" {
		t.Fatalf("expected ERR_ROOT_GUARDRAIL, got %+v", guardRes)
	}
}

func TestRBACWildcardDestructiveExclusionAndABAC(t *testing.T) {
	now := time.Now()
	sub := rbacContracts.Subject{
		ID:             "dev-2",
		TenantID:       "t1",
		MFAVerified:    false,
		Classification: "internal",
		Permissions: []rbacContracts.Permission{
			{Resource: rbacContracts.ResourceServiceCatalog, Action: rbacContracts.ActionAll},
		},
	}
	res := rules.EvaluateAuthorization(rbacContracts.EvaluationRequest{
		Subject:        sub,
		TargetTenantID: "t1",
		Resource:       rbacContracts.ResourceServiceCatalog,
		Action:         rbacContracts.ActionDelete,
	}, now)
	if res.Granted {
		t.Fatal("Case 09: wildcard * must not grant destructive delete action")
	}

	freezeRes := rules.EvaluateAuthorization(rbacContracts.EvaluationRequest{
		Subject:            sub,
		TargetTenantID:     "t1",
		Resource:           rbacContracts.ResourceServiceCatalog,
		Action:             rbacContracts.ActionWrite,
		TargetEnvironment:  rbacContracts.EnvProduction,
		ChangeFreezeActive: true,
	}, now)
	if freezeRes.Granted || freezeRes.ViolationCode != "ERR_CHANGE_FREEZE" {
		t.Fatalf("expected ERR_CHANGE_FREEZE, got %+v", freezeRes)
	}

	mfaRes := rules.EvaluateAuthorization(rbacContracts.EvaluationRequest{
		Subject:           sub,
		TargetTenantID:    "t1",
		Resource:          rbacContracts.ResourceServiceCatalog,
		Action:            rbacContracts.ActionWrite,
		TargetEnvironment: rbacContracts.EnvProduction,
		RequireStepUpMFA:  true,
	}, now)
	if mfaRes.Granted || mfaRes.ViolationCode != "ERR_STEP_UP_MFA_REQUIRED" {
		t.Fatalf("expected ERR_STEP_UP_MFA_REQUIRED, got %+v", mfaRes)
	}
}

func TestRBACRoleHierarchyDAGAndEdgeCases(t *testing.T) {
	cyclic := map[string][]string{
		"dev":     {"lead"},
		"lead":    {"manager"},
		"manager": {"dev"},
	}
	if err := rules.ValidateRoleHierarchyDAG(cyclic); err == nil {
		t.Fatal("P1-04: expected cycle detection error")
	}

	deep := map[string][]string{
		"l1": {"l2"}, "l2": {"l3"}, "l3": {"l4"}, "l4": {"l5"}, "l5": {"l6"},
	}
	if err := rules.ValidateRoleHierarchyDAG(deep); err == nil {
		t.Fatal("Case 06: expected max depth > 5 error")
	}

	if err := rules.EvaluateImpersonation(false, true, false); err == nil {
		t.Fatal("Case 03: expected mTLS error")
	}
	if err := rules.EvaluateDelegationScope(false, "t1", "t1"); err == nil {
		t.Fatal("Case 04: expected confused deputy error")
	}
}

func TestRBACBreakGlassAuditFailSafe(t *testing.T) {
	verifier := adapters.NewJWTTokenVerifierAdapter("secret-key", "idp")
	revStore := adapters.NewMemoryRevocationAdapter()
	svc := service.NewRBACService(verifier, revStore)
	outbox := &mockRBACOutbox{fail: true}
	svc.SetAuditOutbox(outbox)

	_, err := svc.AuthorizeWithContext(context.Background(), rbacContracts.EvaluationRequest{
		Subject:        rbacContracts.Subject{ID: "oncall-1", TenantID: "t1"},
		TargetTenantID: "t1",
		Resource:       rbacContracts.ResourceInfrastructure,
		Action:         rbacContracts.ActionWrite,
		BreakGlass: &rbacContracts.BreakGlassContext{
			TicketID:            "INC-9001",
			TicketApproved:      true,
			TTLSeconds:          1800,
			Tier0Prod:           true,
			DualApprovalGranted: true,
		},
	})
	if err == nil {
		t.Fatal("P0-03: expected break-glass to fail closed when audit outbox fails")
	}
}
