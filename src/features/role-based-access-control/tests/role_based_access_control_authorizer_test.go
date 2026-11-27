/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC AUTHORIZER CAPABILITY TESTS (X-02)

 1. Role:
    - Verifies kernel authz.Authorizer enforcement via RBACAuthorizerProvider.
*/
package tests

import (
	"context"
	"testing"

	"internal-developer-platform/src/features/role-based-access-control/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/role-based-access-control/internal/service"
	"internal-developer-platform/src/shared/kernel"
	"internal-developer-platform/src/shared/kernel/capabilities/authz"
)

func TestAuthorizerCapabilityProvider_CheckEnforcesRBAC(t *testing.T) {
	verifier := adapters.NewJWTTokenVerifierAdapter("authz-test-secret-0123456789", "idp-test")
	revocation := adapters.NewMemoryRevocationAdapter()
	grantRepo := adapters.NewMemoryAccessGrantAdapter()
	svc := service.NewRBACServiceWithDeps(service.RBACServiceDeps{
		TokenVerifier:   verifier,
		RevocationStore: revocation,
		GrantRepo:       grantRepo,
		BootstrapSecret: "authz-bootstrap",
		Environment:     "dev",
	})
	provider := service.NewRBACAuthorizerProvider(svc)

	dec, portErr := provider.Check(context.Background(), authz.CheckRequest{
		Principal: kernel.Principal{
			Subject:  "dev-1",
			TenantID: "tenant-1",
			Roles:    []string{"Developer"},
		},
		TenantID: "tenant-1",
		Resource: string(authz.ResourceServiceCatalog),
		Action:   string(authz.ActionRead),
	})
	if portErr != nil || dec != authz.DecisionPermit {
		t.Fatalf("expected Developer read on service-catalog to PERMIT, got decision=%s err=%v", dec, portErr)
	}

	dec, portErr = provider.Check(context.Background(), authz.CheckRequest{
		Principal: kernel.Principal{
			Subject:  "viewer-1",
			TenantID: "tenant-1",
			Roles:    []string{"Viewer"},
		},
		TenantID: "tenant-2",
		Resource: string(authz.ResourceInfrastructure),
		Action:   string(authz.ActionWrite),
	})
	if portErr == nil || dec != authz.DecisionDeny {
		t.Fatalf("expected Viewer cross-tenant write to DENY, got decision=%s err=%v", dec, portErr)
	}
}
