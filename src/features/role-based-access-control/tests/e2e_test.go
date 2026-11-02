package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	rbacAdapters "internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
)

func TestLiveE2E_FullHTTPServerFlow(t *testing.T) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "18081"
	}
	serverURL := fmt.Sprintf("http://127.0.0.1:%s", port)

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "e2e-automated-regression-secret-32bytes"
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "127.0.0.1:6379"
	}

	redisAdapter := rbacAdapters.NewRedisRevocationAdapter(redisAddr, 2*time.Second)
	verifier := rbacAdapters.NewJWTTokenVerifierAdapter(secret, "internal-developer-platform")
	svc := rbacService.NewRBACService(verifier, redisAdapter)

	// Step 1: Health check through real HTTP listener
	adminPort := os.Getenv("ADMIN_PORT")
	if adminPort == "" {
		adminPort = "19091"
	}
	respHealth, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/healthz", adminPort))
	if err != nil || respHealth.StatusCode != http.StatusOK {
		t.Fatalf("failed healthz check on live server: %v (code: %v)", err, respHealth.StatusCode)
	}

	// Step 2: Unauthenticated POST /api/v1/services must return 401
	respNoAuth, err := http.Post(serverURL+"/api/v1/services", "application/json", bytes.NewBuffer([]byte(`{}`)))
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	if respNoAuth.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for request without auth, got %d", respNoAuth.StatusCode)
	}

	// Step 3: Authenticated Developer registration
	devSub := rbacContracts.Subject{
		ID:        "e2e-dev-engineer-99",
		TenantID:  "tenant-e2e-org",
		Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
		ExpiresAt: time.Now().Add(1 * time.Hour),
		JTI:       fmt.Sprintf("e2e-jti-%d", time.Now().UnixNano()),
	}
	devToken, err := svc.IssueToken(devSub)
	if err != nil {
		t.Fatalf("failed to issue token: %v", err)
	}

	servicePayload := map[string]interface{}{
		"id":        "order-dispatch-api",
		"tenantId":  "tenant-e2e-org",
		"name":      "Order Dispatch API",
		"owner":     "dispatch-team@org.com",
		"lifecycle": "production",
	}
	jsonBytes, _ := json.Marshal(servicePayload)

	reqAuth, _ := http.NewRequest(http.MethodPost, serverURL+"/api/v1/services", bytes.NewBuffer(jsonBytes))
	reqAuth.Header.Set("Authorization", "Bearer "+devToken)
	reqAuth.Header.Set("X-Tenant-ID", "tenant-e2e-org")
	reqAuth.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	respAuth, err := client.Do(reqAuth)
	if err != nil {
		t.Fatalf("authenticated HTTP request failed: %v", err)
	}
	defer respAuth.Body.Close()

	if respAuth.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(respAuth.Body)
		t.Fatalf("expected 201 Created for authenticated dev, got %d: %s", respAuth.StatusCode, string(body))
	}

	// Step 4: Revoke Developer Session in live Redis
	if err := svc.RevokeSubject(reqAuth.Context(), devSub.ID); err != nil {
		t.Fatalf("failed to revoke subject in live Redis: %v", err)
	}

	// Step 5: Same token used again must be instantly blocked with 401
	reqPostRevoke, _ := http.NewRequest(http.MethodPost, serverURL+"/api/v1/services", bytes.NewBuffer(jsonBytes))
	reqPostRevoke.Header.Set("Authorization", "Bearer "+devToken)
	reqPostRevoke.Header.Set("X-Tenant-ID", "tenant-e2e-org")
	reqPostRevoke.Header.Set("Content-Type", "application/json")

	respRevoked, err := client.Do(reqPostRevoke)
	if err != nil {
		t.Fatalf("request failed post revocation: %v", err)
	}
	defer respRevoked.Body.Close()

	if respRevoked.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized after live Redis revocation, got %d", respRevoked.StatusCode)
	}
}
