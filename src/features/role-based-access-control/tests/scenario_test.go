package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/middleware"
	"internal-developer-platform/src/api/rest/v1/routers"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	rbacAdapters "internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
	catalogAdapters "internal-developer-platform/src/features/service-catalog/infrastructure/adapters"
	catalogService "internal-developer-platform/src/features/service-catalog/service"
)

// SetupTestHTTPPipeline creates the full HTTP router with real RBAC middleware and real catalog handlers
func SetupTestHTTPPipeline(revocationStore *rbacAdapters.MemoryRevocationAdapter) (*http.ServeMux, *rbacService.RBACService) {
	jwtVerifier := rbacAdapters.NewJWTTokenVerifierAdapter("scenario-test-secret-key-32bytes", "idp-scenario-issuer")
	rbacSvc := rbacService.NewRBACService(jwtVerifier, revocationStore)
	rbacMW := middleware.NewRBACMiddleware(rbacSvc)

	catalogRepo := catalogAdapters.NewMemoryServiceCatalogAdapter()
	catalogSvc := catalogService.NewServiceCatalogService(catalogRepo)
	catalogHandler := handlers.NewServiceCatalogHandler(catalogSvc)
	catalogRouter := routers.NewServiceCatalogRouter(catalogHandler, rbacMW)

	mux := http.NewServeMux()
	catalogRouter.RegisterRoutes(mux)

	return mux, rbacSvc
}

// --- SCENARIO 1: Cross-Tenant Isolation Enforcement ---
// Real-world: Alice (Payments Team) attempts to register a service inside Core Engineering's tenant
func TestScenario_CrossTenantIsolation(t *testing.T) {
	revocationStore := rbacAdapters.NewMemoryRevocationAdapter()
	mux, rbacSvc := SetupTestHTTPPipeline(revocationStore)

	aliceSubject := rbacContracts.Subject{
		ID:        "alice-payments-dev",
		TenantID:  "tenant-payments",
		Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	aliceToken, err := rbacSvc.IssueToken(aliceSubject)
	if err != nil {
		t.Fatalf("failed to issue token: %v", err)
	}

	// 1. Alice registers service in her OWN tenant -> Expect 201 Created
	ownPayload := map[string]interface{}{
		"id":        "payment-gateway",
		"tenantId":  "tenant-payments",
		"name":      "Payment Gateway Service",
		"owner":     "alice@company.com",
		"lifecycle": "production",
	}
	bodyBytes, _ := json.Marshal(ownPayload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+aliceToken)
	req.Header.Set("X-Tenant-ID", "tenant-payments")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for own tenant registration, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Alice attempts to register service in CORE ENGINEERING tenant -> Expect 403 Forbidden
	foreignPayload := map[string]interface{}{
		"id":        "rogue-core-service",
		"tenantId":  "tenant-core-engineering",
		"name":      "Rogue Core Service",
		"owner":     "alice@company.com",
		"lifecycle": "production",
	}
	foreignBytes, _ := json.Marshal(foreignPayload)

	reqForeign := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewBuffer(foreignBytes))
	reqForeign.Header.Set("Authorization", "Bearer "+aliceToken)
	reqForeign.Header.Set("X-Tenant-ID", "tenant-core-engineering") // Cross-tenant target
	reqForeign.Header.Set("Content-Type", "application/json")
	wForeign := httptest.NewRecorder()

	mux.ServeHTTP(wForeign, reqForeign)
	if wForeign.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for cross-tenant registration attempt, got %d: %s", wForeign.Code, wForeign.Body.String())
	}
}

// --- SCENARIO 2: Read-Only Viewer Privilege Escalation Attempt ---
// Real-world: Bob (Auditor / Viewer) attempts to register a new service
func TestScenario_ViewerPrivilegeEscalation(t *testing.T) {
	revocationStore := rbacAdapters.NewMemoryRevocationAdapter()
	mux, rbacSvc := SetupTestHTTPPipeline(revocationStore)

	bobSubject := rbacContracts.Subject{
		ID:        "bob-compliance-auditor",
		TenantID:  "tenant-payments",
		Roles:     []rbacContracts.RoleName{rbacContracts.RoleViewer},
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	bobToken, _ := rbacSvc.IssueToken(bobSubject)

	payload := map[string]interface{}{
		"id":        "unauthorized-service",
		"tenantId":  "tenant-payments",
		"name":      "Unauthorized Service",
		"owner":     "bob@company.com",
		"lifecycle": "experimental",
	}
	bodyBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+bobToken)
	req.Header.Set("X-Tenant-ID", "tenant-payments")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for viewer write attempt, got %d: %s", w.Code, w.Body.String())
	}
}

// --- SCENARIO 3: Mid-Flight Security Incident Session Revocation ---
// Real-world: Dev credentials compromised; SecOps kills session in revocation store
func TestScenario_ImmediateSessionRevocation(t *testing.T) {
	revocationStore := rbacAdapters.NewMemoryRevocationAdapter()
	mux, rbacSvc := SetupTestHTTPPipeline(revocationStore)

	compromisedDev := rbacContracts.Subject{
		ID:        "charlie-dev-compromised",
		TenantID:  "tenant-payments",
		Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
		ExpiresAt: time.Now().Add(2 * time.Hour),
		JTI:       "session-token-xyz-99",
	}
	token, _ := rbacSvc.IssueToken(compromisedDev)

	// Step 1: Request 1 succeeds before incident response
	payload1 := map[string]interface{}{
		"id":        "service-legit",
		"tenantId":  "tenant-payments",
		"name":      "Legitimate Service",
		"owner":     "charlie@company.com",
		"lifecycle": "development",
	}
	bytes1, _ := json.Marshal(payload1)
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewBuffer(bytes1))
	req1.Header.Set("Authorization", "Bearer "+token)
	req1.Header.Set("X-Tenant-ID", "tenant-payments")
	w1 := httptest.NewRecorder()
	mux.ServeHTTP(w1, req1)

	if w1.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created prior to revocation, got %d", w1.Code)
	}

	// Step 2: SecOps Officer detects compromise and revokes subject immediately
	_ = rbacSvc.RevokeSubject(req1.Context(), "charlie-dev-compromised")

	// Step 3: Request 2 immediately following revocation must be BLOCKED with 401
	payload2 := map[string]interface{}{
		"id":        "service-malicious",
		"tenantId":  "tenant-payments",
		"name":      "Malicious Post-Breach Service",
		"owner":     "attacker@darkweb.io",
		"lifecycle": "production",
	}
	bytes2, _ := json.Marshal(payload2)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewBuffer(bytes2))
	req2.Header.Set("Authorization", "Bearer "+token)
	req2.Header.Set("X-Tenant-ID", "tenant-payments")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized after session revocation, got %d: %s", w2.Code, w2.Body.String())
	}
}

// --- SCENARIO 4: Global Platform Admin Cross-Tenant Intervention ---
// Real-world: Platform SRE performs emergency provisioning across customer tenant
func TestScenario_PlatformAdminIntervention(t *testing.T) {
	revocationStore := rbacAdapters.NewMemoryRevocationAdapter()
	mux, rbacSvc := SetupTestHTTPPipeline(revocationStore)

	sreAdmin := rbacContracts.Subject{
		ID:        "sre-lead-dave",
		TenantID:  "system-internal",
		Roles:     []rbacContracts.RoleName{rbacContracts.RolePlatformAdmin},
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}
	adminToken, _ := rbacSvc.IssueToken(sreAdmin)

	payload := map[string]interface{}{
		"id":        "sre-managed-cache",
		"tenantId":  "customer-tenant-omega",
		"name":      "Managed SRE Cache",
		"owner":     "sre@company.com",
		"lifecycle": "production",
	}
	bytesData, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services", bytes.NewBuffer(bytesData))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("X-Tenant-ID", "customer-tenant-omega") // Admin across tenant
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for Platform Admin intervention, got %d: %s", w.Code, w.Body.String())
	}
}
