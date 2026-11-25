package tests

import (
	"context"
	"errors"
	"net/http"
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

func TestRBACEdgeCases01_02_07_08_10(t *testing.T) {
	leaseOK := rules.EvaluateTaskExecutionLease(rbacContracts.TaskExecutionLeaseRequest{
		TokenExpired:          true,
		IsIdempotentBatchStep: true,
		HasSignedTaskLease:    true,
		LeaseExpired:          false,
	})
	if !leaseOK.Granted || leaseOK.Directive != "ALLOW_LEASE" {
		t.Fatalf("Case 01: expected ALLOW_LEASE, got %+v", leaseOK)
	}
	leaseAbort := rules.EvaluateTaskExecutionLease(rbacContracts.TaskExecutionLeaseRequest{
		TokenExpired:          true,
		IsIdempotentBatchStep: true,
		HasSignedTaskLease:    false,
		RefreshSucceeded:      false,
	})
	if leaseAbort.Granted || leaseAbort.Directive != "ABORT" {
		t.Fatalf("Case 01: expected ABORT, got %+v", leaseAbort)
	}

	introspectCalled := false
	driftDenied := rules.EvaluateAsyncJobRevocationDrift(context.Background(), false, 6*time.Minute, func(ctx context.Context) (bool, error) {
		introspectCalled = true
		return false, nil
	})
	if !introspectCalled || driftDenied.Granted || driftDenied.ViolationCode != "ERR_OIDC_INACTIVE" {
		t.Fatalf("Case 02: expected OIDC introspect deny, got %+v", driftDenied)
	}
	driftAllowed := rules.EvaluateAsyncJobRevocationDrift(context.Background(), false, 2*time.Minute, nil)
	if !driftAllowed.Granted {
		t.Fatalf("Case 02: expected job dispatch within 5m drift, got %+v", driftAllowed)
	}

	splitMutation := rules.EvaluateRegionalSplitBrain(false, true, 10*time.Minute)
	if splitMutation.Granted || splitMutation.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("Case 07: expected 503 for role mutation during partition, got %+v", splitMutation)
	}
	splitDeploy := rules.EvaluateRegionalSplitBrain(false, false, 90*time.Minute)
	if !splitDeploy.Granted || !splitDeploy.Degraded || splitDeploy.WarningHeader == "" {
		t.Fatalf("Case 07: expected degraded allow within 2h grace window, got %+v", splitDeploy)
	}
	splitExpired := rules.EvaluateRegionalSplitBrain(false, false, 3*time.Hour)
	if splitExpired.Granted || splitExpired.HTTPStatus != http.StatusForbidden {
		t.Fatalf("Case 07: expected fail closed when snapshot > 2h, got %+v", splitExpired)
	}

	if err := rules.EvaluateExclusiveRoleClaim(1); err != nil {
		t.Fatalf("Case 08: expected exclusive role claim success on 1 row, got %v", err)
	}
	if err := rules.EvaluateExclusiveRoleClaim(0); err == nil {
		t.Fatal("Case 08: expected conflict error on 0 rows affected")
	}

	now := time.Now()
	hbTerminate := rules.EvaluateStreamingSessionHeartbeat(now.Add(-1*time.Minute), now, now.Add(-31*time.Minute))
	if !hbTerminate.Terminate {
		t.Fatalf("Case 10: expected terminate when JIT grant expired, got %+v", hbTerminate)
	}
	hbActive := rules.EvaluateStreamingSessionHeartbeat(now.Add(10*time.Minute), now, now.Add(-5*time.Minute))
	if hbActive.Terminate {
		t.Fatalf("Case 10: expected active session when JIT grant valid, got %+v", hbActive)
	}
}

func TestRBACPermitCacheAndAuditOutbox(t *testing.T) {
	verifier := adapters.NewJWTTokenVerifierAdapter("secret-key", "idp")
	revStore := adapters.NewMemoryRevocationAdapter()
	memBindings := adapters.NewMemoryUserBindingAdapter()
	memOutbox := adapters.NewMemoryRBACAuditOutboxAdapter()

	svc := service.NewRBACService(verifier, revStore)
	svc.SetUserBindingRepository(memBindings)
	svc.SetAuditOutbox(memOutbox)

	ctx := context.Background()
	sub := rbacContracts.Subject{
		ID:          "dev-cache-1",
		TenantID:    "t1",
		Roles:       []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
		MFAVerified: true,
	}

	res, err := svc.AuthorizeWithContext(ctx, rbacContracts.EvaluationRequest{
		Subject:           sub,
		TargetTenantID:    "t1",
		Resource:          rbacContracts.ResourceServiceCatalog,
		Action:            rbacContracts.ActionRead,
		TargetEnvironment: rbacContracts.EnvStaging,
	})
	if err != nil || !res.Granted {
		t.Fatalf("expected permit, got %+v err=%v", res, err)
	}
	if svc.PermitCacheSize() != 1 {
		t.Fatalf("expected 1 cached permit entry, got %d", svc.PermitCacheSize())
	}

	_, err = svc.AuthorizeWithContext(ctx, rbacContracts.EvaluationRequest{
		Subject:           sub,
		TargetTenantID:    "t1",
		Resource:          rbacContracts.ResourceServiceCatalog,
		Action:            rbacContracts.ActionWrite,
		TargetEnvironment: rbacContracts.EnvProduction,
	})
	if err != nil {
		t.Fatalf("expected production mutation permit, got err=%v", err)
	}
	if svc.PermitCacheSize() != 1 {
		t.Fatalf("Case 05: production mutation must bypass permit cache, got size %d", svc.PermitCacheSize())
	}

	_, err = svc.AuthorizeWithContext(ctx, rbacContracts.EvaluationRequest{
		Subject:           sub,
		TargetTenantID:    "t1",
		Resource:          rbacContracts.ResourceSecrets,
		Action:            rbacContracts.ActionDelete,
		TargetEnvironment: rbacContracts.EnvStaging,
	})
	if err == nil {
		t.Fatal("expected deny for secrets delete")
	}
	if svc.PermitCacheSize() != 1 {
		t.Fatalf("Tier 6: DENY must never be cached, got size %d", svc.PermitCacheSize())
	}

	_ = memBindings.BindUserRole(ctx, rbacContracts.UserBindingRecord{
		SubjectID: sub.ID,
		TenantID:  "t1",
		RoleName:  rbacContracts.RoleDeveloper,
	})
	if err := svc.UnbindUserRole(ctx, "t1", sub.ID, rbacContracts.RoleDeveloper); err != nil {
		t.Fatalf("unbind failed: %v", err)
	}
	if svc.PermitCacheSize() != 0 {
		t.Fatalf("expected cache invalidated on UnbindUserRole, got size %d", svc.PermitCacheSize())
	}

	_, _ = svc.AuthorizeWithContext(ctx, rbacContracts.EvaluationRequest{
		Subject:           sub,
		TargetTenantID:    "t1",
		Resource:          rbacContracts.ResourceServiceCatalog,
		Action:            rbacContracts.ActionRead,
		TargetEnvironment: rbacContracts.EnvStaging,
	})
	if svc.PermitCacheSize() != 1 {
		t.Fatalf("expected cache repopulated, got size %d", svc.PermitCacheSize())
	}
	if err := svc.RevokeSubject(ctx, sub.ID); err != nil {
		t.Fatalf("revoke subject failed: %v", err)
	}
	if svc.PermitCacheSize() != 0 {
		t.Fatalf("expected cache invalidated on RevokeSubject, got size %d", svc.PermitCacheSize())
	}

	events := memOutbox.ListEvents()
	if len(events) < 4 {
		t.Fatalf("expected at least 4 audit outbox events, got %d", len(events))
	}
	pgOutbox := adapters.NewPostgresRBACAuditOutboxAdapter(nil)
	if err := pgOutbox.EnqueueAuditEvent(ctx, events[0]); err == nil {
		t.Fatal("expected error from PostgresRBACAuditOutboxAdapter with nil DB")
	}
}
