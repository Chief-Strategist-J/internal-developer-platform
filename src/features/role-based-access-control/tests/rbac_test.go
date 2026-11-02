package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"internal-developer-platform/src/api/rest/v1/middleware"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/features/role-based-access-control/domain/rules"
	rbacAdapters "internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
)

func TestEvaluateAuthorization_Rules(t *testing.T) {
	now := time.Now()

	// 1. Missing Subject
	res := rules.EvaluateAuthorization(rbacContracts.EvaluationRequest{
		Subject:  rbacContracts.Subject{},
		Resource: rbacContracts.ResourceServiceCatalog,
		Action:   rbacContracts.ActionRead,
	}, now)
	if res.Granted {
		t.Errorf("expected denial for empty subject")
	}

	// 2. Expired Token
	res = rules.EvaluateAuthorization(rbacContracts.EvaluationRequest{
		Subject: rbacContracts.Subject{
			ID:        "user-1",
			ExpiresAt: now.Add(-1 * time.Hour),
		},
		Resource: rbacContracts.ResourceServiceCatalog,
		Action:   rbacContracts.ActionRead,
	}, now)
	if res.Granted {
		t.Errorf("expected denial for expired token")
	}

	// 3. Platform Admin Global Access (even across tenants)
	res = rules.EvaluateAuthorization(rbacContracts.EvaluationRequest{
		Subject: rbacContracts.Subject{
			ID:        "admin-1",
			TenantID:  "tenant-a",
			Roles:     []rbacContracts.RoleName{rbacContracts.RolePlatformAdmin},
			ExpiresAt: now.Add(1 * time.Hour),
		},
		TargetTenantID: "tenant-b",
		Resource:       rbacContracts.ResourceServiceCatalog,
		Action:         rbacContracts.ActionWrite,
	}, now)
	if !res.Granted {
		t.Errorf("expected global admin to be granted cross-tenant access")
	}

	// 4. Developer Tenant Isolation Violation
	res = rules.EvaluateAuthorization(rbacContracts.EvaluationRequest{
		Subject: rbacContracts.Subject{
			ID:        "dev-1",
			TenantID:  "tenant-a",
			Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
			ExpiresAt: now.Add(1 * time.Hour),
		},
		TargetTenantID: "tenant-b",
		Resource:       rbacContracts.ResourceServiceCatalog,
		Action:         rbacContracts.ActionWrite,
	}, now)
	if res.Granted {
		t.Errorf("expected denial for cross-tenant access attempt by developer")
	}

	// 5. Developer Valid Tenant Match & Write Action
	res = rules.EvaluateAuthorization(rbacContracts.EvaluationRequest{
		Subject: rbacContracts.Subject{
			ID:        "dev-1",
			TenantID:  "tenant-a",
			Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
			ExpiresAt: now.Add(1 * time.Hour),
		},
		TargetTenantID: "tenant-a",
		Resource:       rbacContracts.ResourceServiceCatalog,
		Action:         rbacContracts.ActionWrite,
	}, now)
	if !res.Granted {
		t.Errorf("expected developer to have write access to service-catalog")
	}

	// 6. Viewer Forbidden from Write Action
	res = rules.EvaluateAuthorization(rbacContracts.EvaluationRequest{
		Subject: rbacContracts.Subject{
			ID:        "viewer-1",
			TenantID:  "tenant-a",
			Roles:     []rbacContracts.RoleName{rbacContracts.RoleViewer},
			ExpiresAt: now.Add(1 * time.Hour),
		},
		TargetTenantID: "tenant-a",
		Resource:       rbacContracts.ResourceServiceCatalog,
		Action:         rbacContracts.ActionWrite,
	}, now)
	if res.Granted {
		t.Errorf("expected viewer to be denied write access to service-catalog")
	}
}

func TestRBACService_TokenLifecycleAndRevocation(t *testing.T) {
	ctx := context.Background()
	revocationStore := rbacAdapters.NewMemoryRevocationAdapter()
	tokenVerifier := rbacAdapters.NewJWTTokenVerifierAdapter("test-secret-key-12345", "test-issuer")
	svc := rbacService.NewRBACService(tokenVerifier, revocationStore)

	sub := rbacContracts.Subject{
		ID:        "dev-42",
		TenantID:  "tenant-core",
		Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
		ExpiresAt: time.Now().Add(2 * time.Hour),
		JTI:       "token-jti-xyz",
	}

	tokenStr, err := svc.IssueToken(sub)
	if err != nil {
		t.Fatalf("failed to issue token: %v", err)
	}

	// Authenticate valid token
	authSub, err := svc.AuthenticateToken(ctx, tokenStr)
	if err != nil {
		t.Fatalf("expected successful authentication: %v", err)
	}
	if authSub.ID != "dev-42" {
		t.Errorf("expected subject id dev-42, got %s", authSub.ID)
	}

	// Revoke subject in revocation store
	if err := svc.RevokeSubject(ctx, "dev-42"); err != nil {
		t.Fatalf("failed to revoke subject: %v", err)
	}

	// Subsequent authentication must fail
	_, err = svc.AuthenticateToken(ctx, tokenStr)
	if err == nil {
		t.Fatalf("expected authentication to fail for revoked subject")
	}
}

func TestRBACMiddleware_HTTPPipeline(t *testing.T) {
	revocationStore := rbacAdapters.NewMemoryRevocationAdapter()
	tokenVerifier := rbacAdapters.NewJWTTokenVerifierAdapter("test-secret-key-12345", "test-issuer")
	svc := rbacService.NewRBACService(tokenVerifier, revocationStore)
	mw := middleware.NewRBACMiddleware(svc)

	nextCalled := false
	dummyHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	protected := mw.RequirePermission(rbacContracts.ResourceServiceCatalog, rbacContracts.ActionWrite, dummyHandler)

	// 1. Missing Authorization header -> 401
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/services", nil)
	w1 := httptest.NewRecorder()
	protected(w1, req1)
	if w1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized for missing auth header, got %d", w1.Code)
	}

	// 2. Viewer Token attempting Write -> 403
	viewerSub := rbacContracts.Subject{
		ID:        "viewer-10",
		TenantID:  "payments-team",
		Roles:     []rbacContracts.RoleName{rbacContracts.RoleViewer},
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	viewerToken, _ := svc.IssueToken(viewerSub)

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/services", nil)
	req2.Header.Set("Authorization", "Bearer "+viewerToken)
	req2.Header.Set("X-Tenant-ID", "payments-team")
	w2 := httptest.NewRecorder()
	protected(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for viewer write attempt, got %d", w2.Code)
	}

	// 3. Developer Token with valid Tenant -> 200
	devSub := rbacContracts.Subject{
		ID:        "dev-10",
		TenantID:  "payments-team",
		Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	devToken, _ := svc.IssueToken(devSub)

	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/services", nil)
	req3.Header.Set("Authorization", "Bearer "+devToken)
	req3.Header.Set("X-Tenant-ID", "payments-team")
	w3 := httptest.NewRecorder()
	protected(w3, req3)
	if w3.Code != http.StatusOK {
		t.Errorf("expected 200 OK for authorized developer write, got %d", w3.Code)
	}
	if !nextCalled {
		t.Errorf("expected inner handler to be called")
	}
}
