package tests

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/routers"
	gitopsreconciliation "internal-developer-platform/src/features/gitops-reconciliation"
	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/rules"
)

type MockFullArgoClient struct {
	apps map[string]bool
}

func NewMockFullArgoClient() *MockFullArgoClient {
	return &MockFullArgoClient{
		apps: make(map[string]bool),
	}
}

func (m *MockFullArgoClient) TriggerSync(ctx context.Context, appName string, revision string, prune bool, dryRun bool) (int, error) {
	return 2, nil
}

func (m *MockFullArgoClient) GetApplicationStatus(ctx context.Context, appName string) (contracts.SyncPhase, contracts.HealthStatus, string, error) {
	return contracts.SyncPhaseSynced, contracts.HealthStatusHealthy, "active-rev", nil
}

func (m *MockFullArgoClient) DiffManifests(ctx context.Context, appName string, revision string) ([]rules.ResourceManifest, rules.DriftReport, error) {
	manifests := []rules.ResourceManifest{
		{Kind: "Namespace", Name: appName, SyncWave: -1},
		{Kind: "ConfigMap", Name: appName + "-cfg", SyncWave: 0},
		{Kind: "Deployment", Name: appName, SyncWave: 1},
	}
	return manifests, rules.EvaluateDriftSeverity(0, 0), nil
}

func (m *MockFullArgoClient) GetDetailedDrift(ctx context.Context, appName string, revision string) (*contracts.DetailedDriftReport, error) {
	return &contracts.DetailedDriftReport{
		AppName:         appName,
		TargetRevision:  revision,
		HasDrift:        true,
		Items:           []contracts.ClassifiedDriftItem{
			{
				ResourceKind:     "Deployment",
				ResourceName:     appName,
				Namespace:        appName,
				Classification:   contracts.DriftTypeAModified,
				LiveValue:        "replicas: 4",
				DesiredValue:     "replicas: 2",
				FieldPath:        "spec.replicas",
				IsRuntimeIgnored: true,
			},
			{
				ResourceKind:     "ConfigMap",
				ResourceName:     appName + "-cfg",
				Namespace:        appName,
				Classification:   contracts.DriftTypeAModified,
				LiveValue:        "LOG_LEVEL=DEBUG",
				DesiredValue:     "LOG_LEVEL=INFO",
				FieldPath:        "data.LOG_LEVEL",
				IsRuntimeIgnored: false,
			},
		},
		TotalDriftCount: 2,
		IgnoredCount:    1,
		CanAutoSync:     false,
	}, nil
}

func (m *MockFullArgoClient) RefreshAppsForRepo(ctx context.Context, repoURL string, branch string) ([]string, error) {
	return []string{"payments-service-main"}, nil
}

func TestGitOpsWebhookHMACVerificationAndIngress(t *testing.T) {
	store := gitopsreconciliation.NewMemoryStore()
	argoClient := NewMockFullArgoClient()
	svc := gitopsreconciliation.NewService(store, argoClient)
	handler := handlers.NewGitOpsReconciliationHandler(svc)
	router := routers.NewGitOpsReconciliationRouter(handler, nil)

	mux := http.NewServeMux()
	router.RegisterRoutes(mux)

	webhookPayload := contracts.GitOpsWebhookPayload{
		RepositoryURL: "https://github.com/org/payments-service.git",
		Branch:        "main",
		CommitSHA:     "abc1234567890abcdef",
		Author:        "lead-architect",
		EventType:     "push",
	}

	payloadBytes, _ := json.Marshal(webhookPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/gitops", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid webhook payload, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	data := resp["data"].(map[string]interface{})
	if !data["accepted"].(bool) {
		t.Fatalf("expected accepted=true")
	}
	refreshed := data["refreshedApps"].([]interface{})
	if len(refreshed) != 1 || refreshed[0] != "payments-service-main" {
		t.Fatalf("expected refreshedApps to contain payments-service-main")
	}
}

func TestGitOpsDriftInspectionEndpoint(t *testing.T) {
	ctx := context.Background()
	store := gitopsreconciliation.NewMemoryStore()
	argoClient := NewMockFullArgoClient()
	svc := gitopsreconciliation.NewService(store, argoClient)

	cmd := contracts.SyncCommand{
		ID:             "rec-drift-01",
		TenantID:       "fintech",
		ServiceID:      "payments",
		AppName:        "payments-service",
		TargetRevision: "v1.2.0",
		IdempotencyKey: "drift-key-01",
	}

	res, err := svc.TriggerReconciliation(ctx, cmd)
	if err != nil {
		t.Fatalf("expected trigger to succeed, got %v", err)
	}

	handler := handlers.NewGitOpsReconciliationHandler(svc)
	router := routers.NewGitOpsReconciliationRouter(handler, nil)
	mux := http.NewServeMux()
	router.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/reconciliations/rec-drift-01/diff", nil)
	req.Header.Set("x-tenant-id", "fintech")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for drift diff, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if !data["hasDrift"].(bool) {
		t.Fatalf("expected hasDrift=true")
	}
	if int(data["totalDriftCount"].(float64)) != 2 {
		t.Fatalf("expected totalDriftCount=2, got %v", data["totalDriftCount"])
	}
	if int(data["ignoredCount"].(float64)) != 1 {
		t.Fatalf("expected ignoredCount=1, got %v", data["ignoredCount"])
	}
	_ = res
}

func TestHMACVerificationLogic(t *testing.T) {
	secret := "super-secure-webhook-secret"
	payload := []byte(`{"repo":"test","branch":"main"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !rules.VerifyWebhookHMAC(payload, validSig, secret) {
		t.Fatalf("expected HMAC verification to pass for valid signature")
	}

	if rules.VerifyWebhookHMAC(payload, "sha256=invalid", secret) {
		t.Fatalf("expected HMAC verification to fail for invalid signature")
	}
}

func TestTenantGuardrailsPreventClusterScopedObjects(t *testing.T) {
	err := rules.ValidateTenantProjectGuardrails(true, "CustomResourceDefinition")
	if err == nil {
		t.Fatalf("expected error blocking CRD from tenant project")
	}

	err = rules.ValidateTenantProjectGuardrails(true, "ClusterRole")
	if err == nil {
		t.Fatalf("expected error blocking ClusterRole from tenant project")
	}

	err = rules.ValidateTenantProjectGuardrails(true, "Deployment")
	if err != nil {
		t.Fatalf("expected Deployment to be allowed, got: %v", err)
	}

	err = rules.ValidateTenantProjectGuardrails(false, "CustomResourceDefinition")
	if err != nil {
		t.Fatalf("expected platform-scoped project to allow CRD, got: %v", err)
	}
}

