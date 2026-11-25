package tests

import (
	"context"
	"testing"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
)

func TestRoleBasedAccessControlScenario_JITLifecycleAndRevocation(t *testing.T) {
	ctx := context.Background()
	verifier := adapters.NewJWTTokenVerifierAdapter("scenario-secret-key", "idp-scenario")
	revocation := adapters.NewMemoryRevocationAdapter()
	grantRepo := adapters.NewMemoryAccessGrantAdapter()
	bindingRepo := adapters.NewMemoryUserBindingAdapter()
	auditOutbox := adapters.NewMemoryRBACAuditOutboxAdapter()

	svc := rbacService.NewRBACService(verifier, revocation, grantRepo)
	svc.SetUserBindingRepository(bindingRepo)
	svc.SetAuditOutbox(auditOutbox)

	err := svc.BindUserRole(ctx, rbacContracts.UserBindingRecord{
		TenantID:  "tenant-scen-1",
		SubjectID: "user-scen-1",
		RoleName:  rbacContracts.RoleDeveloper,
	})
	if err != nil {
		t.Fatalf("BindUserRole failed: %v", err)
	}

	roles, err := svc.GetUserRoles(ctx, "tenant-scen-1", "user-scen-1")
	if err != nil || len(roles) != 1 || roles[0] != rbacContracts.RoleDeveloper {
		t.Fatalf("GetUserRoles failed: roles=%v err=%v", roles, err)
	}

	token, err := svc.IssueToken(rbacContracts.Subject{
		ID:        "user-scen-1",
		TenantID:  "tenant-scen-1",
		ExpiresAt: time.Now().Add(1 * time.Hour),
		JTI:       "jti-scen-1",
	})
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	if _, err := svc.AuthenticateAndAuthorize(ctx, token, "tenant-scen-1", rbacContracts.ResourceServiceCatalog, rbacContracts.ActionRead); err != nil {
		t.Fatalf("expected authorized access before revocation, got: %v", err)
	}

	if err := svc.RevokeSubject(ctx, "user-scen-1"); err != nil {
		t.Fatalf("RevokeSubject failed: %v", err)
	}

	if _, err := svc.AuthenticateAndAuthorize(ctx, token, "tenant-scen-1", rbacContracts.ResourceServiceCatalog, rbacContracts.ActionRead); err == nil {
		t.Fatal("expected unauthorized error after subject revocation")
	}

	if err := svc.UnbindUserRole(ctx, "tenant-scen-1", "user-scen-1", rbacContracts.RoleDeveloper); err != nil {
		t.Fatalf("UnbindUserRole failed: %v", err)
	}
}

func TestRoleBasedAccessControlScenario_AdversarialInputsAndEdgeCases(t *testing.T) {
	ctx := context.Background()
	verifier := adapters.NewJWTTokenVerifierAdapter("adversarial-secret", "idp-adversarial")
	revocation := adapters.NewMemoryRevocationAdapter()
	grantRepo := adapters.NewMemoryAccessGrantAdapter()
	bindingRepo := adapters.NewMemoryUserBindingAdapter()

	svc := rbacService.NewRBACService(verifier, revocation, grantRepo)
	svc.SetUserBindingRepository(bindingRepo)

	t.Run("EmptyAndMalformedTokens", func(t *testing.T) {
		for _, tok := range []string{"", "not-a-jwt", "a.b.c"} {
			if _, err := svc.AuthenticateToken(ctx, tok); err == nil {
				t.Fatalf("expected error for malformed token %q", tok)
			}
		}
	})

	t.Run("ExpiredTokenRejected", func(t *testing.T) {
		token, err := svc.IssueToken(rbacContracts.Subject{
			ID:        "user-exp",
			TenantID:  "tenant-a",
			Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
			ExpiresAt: time.Now().Add(-5 * time.Minute),
			JTI:       "jti-exp",
		})
		if err != nil {
			t.Fatalf("IssueToken failed: %v", err)
		}
		if _, err := svc.AuthenticateAndAuthorize(ctx, token, "tenant-a", rbacContracts.ResourceServiceCatalog, rbacContracts.ActionRead); err == nil {
			t.Fatal("expected error for expired token")
		}
	})

	t.Run("CrossTenantEscalationBlocked", func(t *testing.T) {
		token, err := svc.IssueToken(rbacContracts.Subject{
			ID:        "user-a",
			TenantID:  "tenant-a",
			Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
			ExpiresAt: time.Now().Add(1 * time.Hour),
			JTI:       "jti-a",
		})
		if err != nil {
			t.Fatalf("IssueToken failed: %v", err)
		}
		if _, err := svc.AuthenticateAndAuthorize(ctx, token, "tenant-b", rbacContracts.ResourceServiceCatalog, rbacContracts.ActionRead); err == nil {
			t.Fatal("expected forbidden error for cross-tenant access")
		}
	})

	t.Run("ValidationBoundariesOnRepositories", func(t *testing.T) {
		if err := svc.BindUserRole(ctx, rbacContracts.UserBindingRecord{}); err == nil {
			t.Fatal("expected validation error on empty BindUserRole")
		}
		if err := svc.UnbindUserRole(ctx, "", "u1", rbacContracts.RoleViewer); err == nil {
			t.Fatal("expected validation error on empty tenantId in UnbindUserRole")
		}
		if _, err := svc.GetUserRoles(ctx, "t1", ""); err == nil {
			t.Fatal("expected validation error on empty subjectId in GetUserRoles")
		}
		if _, err := svc.ListActiveGrants(ctx, "", "u1"); err == nil {
			t.Fatal("expected validation error on empty tenantId in ListActiveGrants")
		}
		if err := svc.RevokeJITAccess(ctx, "t1", ""); err == nil {
			t.Fatal("expected validation error on empty grantId in RevokeJITAccess")
		}
	})

	t.Run("GovernanceGuardrails", func(t *testing.T) {
		cyclicEdges := map[string][]string{
			"role-a": {"role-b"},
			"role-b": {"role-a"},
		}
		if err := svc.ValidateRoleHierarchy(ctx, cyclicEdges); err == nil {
			t.Fatal("expected error for cyclic role hierarchy")
		}
		if err := svc.VerifyImpersonation(ctx, false, true, false); err == nil {
			t.Fatal("expected error for impersonation without mTLS")
		}
		if err := svc.VerifyDelegationScope(ctx, true, "tenant-a", "tenant-b"); err == nil {
			t.Fatal("expected error for cross-tenant delegation")
		}
	})
}
