package tests

import (
	"context"
	"testing"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/internal/contracts"
	"internal-developer-platform/src/features/role-based-access-control/internal/domain/rules"
	"internal-developer-platform/src/features/role-based-access-control/internal/infrastructure/adapters"
	rbacService "internal-developer-platform/src/features/role-based-access-control/internal/service"
)

func BenchmarkRBAC_EvaluateAuthorization(b *testing.B) {
	now := time.Now()
	req := rbacContracts.EvaluationRequest{
		Subject: rbacContracts.Subject{
			ID:        "bench-user-1",
			TenantID:  "bench-tenant",
			Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
			ExpiresAt: now.Add(1 * time.Hour),
		},
		TargetTenantID: "bench-tenant",
		Resource:       rbacContracts.ResourceServiceCatalog,
		Action:         rbacContracts.ActionRead,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res := rules.EvaluateAuthorization(req, now)
		if !res.Granted {
			b.Fatal("expected authorization to be granted")
		}
	}
}

func BenchmarkRBAC_AuthenticateAndAuthorize(b *testing.B) {
	ctx := context.Background()
	verifier := adapters.NewJWTTokenVerifierAdapter("bench-secret-key", "idp-bench")
	revocation := adapters.NewMemoryRevocationAdapter()
	svc := rbacService.NewRBACService(verifier, revocation)

	token, err := svc.IssueToken(rbacContracts.Subject{
		ID:        "bench-user-2",
		TenantID:  "bench-tenant",
		Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
		ExpiresAt: time.Now().Add(1 * time.Hour),
		JTI:       "bench-jti-1",
	})
	if err != nil {
		b.Fatalf("IssueToken failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.AuthenticateAndAuthorize(ctx, token, "bench-tenant", rbacContracts.ResourceServiceCatalog, rbacContracts.ActionRead); err != nil {
			b.Fatalf("AuthenticateAndAuthorize failed: %v", err)
		}
	}
}

func BenchmarkRBAC_ValidateRoleHierarchyDAG(b *testing.B) {
	edges := map[string][]string{
		"platform-admin": {"operator"},
		"operator":       {"developer"},
		"developer":      {"viewer"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := rules.ValidateRoleHierarchyDAG(edges); err != nil {
			b.Fatalf("ValidateRoleHierarchyDAG failed: %v", err)
		}
	}
}

func BenchmarkRBAC_RequestJITAccess(b *testing.B) {
	ctx := context.Background()
	verifier := adapters.NewJWTTokenVerifierAdapter("bench-secret-key", "idp-bench")
	revocation := adapters.NewMemoryRevocationAdapter()
	grantRepo := adapters.NewMemoryAccessGrantAdapter()
	svc := rbacService.NewRBACService(verifier, revocation, grantRepo)

	req := rbacContracts.AccessGrantRequest{
		ID:              "bench-grant-1",
		TenantID:        "bench-tenant",
		SubjectID:       "bench-user-3",
		TargetNamespace: "prod-ns",
		Role:            string(rbacContracts.RoleDeveloper),
		Reason:          "benchmark jit access",
		DurationSeconds: 1800,
		ApproverRole:    string(rbacContracts.RolePlatformAdmin),
		IdempotencyKey:  "bench-idem-1",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.RequestJITAccess(ctx, req); err != nil {
			b.Fatalf("RequestJITAccess failed: %v", err)
		}
	}
}
