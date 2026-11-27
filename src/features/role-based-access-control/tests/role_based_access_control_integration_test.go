package tests

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/internal/contracts"
	rbacAdapters "internal-developer-platform/src/features/role-based-access-control/internal/infrastructure/adapters"
	rbacService "internal-developer-platform/src/features/role-based-access-control/internal/service"
)

func TestRoleBasedAccessControlIntegration_LiveRedis(t *testing.T) {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:6379", 1*time.Second)
	if err != nil {
		t.Skip("Live Redis 7.4 unreachable")
		return
	}
	conn.Close()

	ctx := context.Background()
	adapter := rbacAdapters.NewRedisRevocationAdapter("127.0.0.1:6379", 2*time.Second)

	testSub := fmt.Sprintf("sub-canonical-%d", time.Now().UnixNano())
	testJTI := fmt.Sprintf("jti-canonical-%d", time.Now().UnixNano())

	revoked, err := adapter.IsRevoked(ctx, testSub, testJTI)
	if err != nil || revoked {
		t.Fatalf("expected not revoked initially, got revoked=%v, err=%v", revoked, err)
	}

	if err := adapter.RevokeSubject(ctx, testSub); err != nil {
		t.Fatalf("failed to revoke subject: %v", err)
	}

	revoked, err = adapter.IsRevoked(ctx, testSub, "")
	if err != nil || !revoked {
		t.Fatalf("expected subject revoked in Redis, got %v", revoked)
	}

	if err := adapter.RevokeJTI(ctx, testJTI); err != nil {
		t.Fatalf("failed to revoke JTI: %v", err)
	}

	revoked, err = adapter.IsRevoked(ctx, "", testJTI)
	if err != nil || !revoked {
		t.Fatalf("expected JTI revoked in Redis, got %v", revoked)
	}
}

func TestRoleBasedAccessControl_MemoryGrantAdapterAndService(t *testing.T) {
	ctx := context.Background()
	grantRepo := rbacAdapters.NewMemoryAccessGrantAdapter()
	svc := rbacService.NewRBACService(nil, nil, grantRepo)

	req := rbacContracts.AccessGrantRequest{
		ID:              "grant-mem-01",
		TenantID:        "tenant-gov",
		SubjectID:       "usr-bob",
		TargetNamespace: "tax-prod",
		Role:            "DeveloperDebug",
		Reason:          "investigating memory leak on ingestion worker",
		DurationSeconds: 3600,
		ApproverRole:    "SecurityLead",
		IdempotencyKey:  "idemp-mem-01",
	}

	record, err := svc.RequestJITAccess(ctx, req)
	if err != nil {
		t.Fatalf("failed to request JIT access: %v", err)
	}

	if record.Status != "ACTIVE" {
		t.Fatalf("expected status ACTIVE, got %s", record.Status)
	}

	active, err := svc.ListActiveGrants(ctx, "tenant-gov", "usr-bob")
	if err != nil || len(active) != 1 {
		t.Fatalf("expected 1 active grant, got %d, err: %v", len(active), err)
	}

	if err := svc.RevokeJITAccess(ctx, "tenant-gov", "grant-mem-01"); err != nil {
		t.Fatalf("failed to revoke grant: %v", err)
	}

	activeAfterRevoke, err := svc.ListActiveGrants(ctx, "tenant-gov", "usr-bob")
	if err != nil || len(activeAfterRevoke) != 0 {
		t.Fatalf("expected 0 active grants after revocation, got %d", len(activeAfterRevoke))
	}
}

func TestRoleBasedAccessControl_MemoryUserBindingAdapterAndService(t *testing.T) {
	ctx := context.Background()
	bindingRepo := rbacAdapters.NewMemoryUserBindingAdapter()
	svc := rbacService.NewRBACService(nil, nil)
	svc.SetUserBindingRepository(bindingRepo)

	tenantID := "tenant-alpha"
	subjectID := "usr-alice"

	roles, err := svc.GetUserRoles(ctx, tenantID, subjectID)
	if err != nil {
		t.Fatalf("unexpected error getting initial roles: %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("expected 0 roles, got %d", len(roles))
	}

	binding := rbacContracts.UserBindingRecord{
		SubjectID: subjectID,
		TenantID:  tenantID,
		RoleName:  rbacContracts.RoleDeveloper,
		CreatedAt: time.Now(),
	}
	if err := svc.BindUserRole(ctx, binding); err != nil {
		t.Fatalf("failed to bind user role: %v", err)
	}

	roles, err = svc.GetUserRoles(ctx, tenantID, subjectID)
	if err != nil || len(roles) != 1 || roles[0] != rbacContracts.RoleDeveloper {
		t.Fatalf("expected role Developer, got %v, err: %v", roles, err)
	}

	if err := svc.UnbindUserRole(ctx, tenantID, subjectID, rbacContracts.RoleDeveloper); err != nil {
		t.Fatalf("failed to unbind user role: %v", err)
	}

	roles, err = svc.GetUserRoles(ctx, tenantID, subjectID)
	if err != nil || len(roles) != 0 {
		t.Fatalf("expected 0 roles after unbind, got %d", len(roles))
	}
}
