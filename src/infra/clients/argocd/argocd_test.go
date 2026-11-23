package argocd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"internal-developer-platform/src/infra/clients/resilience"
)

func TestArgoCDClientTriggerSyncSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/applications/order-service/sync" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing or invalid authorization header")
		}
		if r.Header.Get("x-idempotency-key") == "" {
			t.Errorf("missing x-idempotency-key header")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"metadata": {"name": "order-service", "namespace": "argocd"},
			"status": {
				"sync": {"status": "Synced", "revision": "rev-123"},
				"health": {"status": "Healthy", "message": "all pods ready"},
				"operationState": {
					"phase": "Succeeded",
					"syncResult": {
						"resources": [
							{"kind": "Deployment", "name": "order-service", "status": "Synced", "syncWave": 1}
						]
					}
				}
			}
		}`))
	}))
	defer server.Close()

	cfg := ArgoCDClientConfig{
		BaseURL:   server.URL,
		AuthToken: "test-token",
		Timeout:   5 * time.Second,
	}
	resCfg := resilience.DefaultResilienceConfig()
	client := NewArgoCDClient(cfg, resCfg)

	resp, err := client.TriggerSync(context.Background(), "order-service", "rev-123", true, false)
	if err != nil {
		t.Fatalf("unexpected error on trigger sync: %v", err)
	}

	if resp.Status.Sync.Status != SyncPhaseSynced {
		t.Errorf("expected Synced, got %s", resp.Status.Sync.Status)
	}
	if resp.Status.Health.Status != HealthStatusHealthy {
		t.Errorf("expected Healthy, got %s", resp.Status.Health.Status)
	}
	if len(resp.Status.OperationState.SyncResult.Resources) != 1 {
		t.Errorf("expected 1 resource in sync result")
	}
}

func TestArgoCDClientGetApplicationNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error": "application not found"}`))
	}))
	defer server.Close()

	cfg := ArgoCDClientConfig{
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	}
	resCfg := resilience.DefaultResilienceConfig()
	client := NewArgoCDClient(cfg, resCfg)

	_, err := client.GetApplication(context.Background(), "missing-app")
	if err == nil {
		t.Fatalf("expected 404 error, got nil")
	}
}

func TestArgoCDClientResilienceBudget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := ArgoCDClientConfig{
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	}
	resCfg := resilience.DefaultResilienceConfig()
	resCfg.MinDeadlineMargin = 50 * time.Millisecond
	resCfg.ConnectTimeout = 100 * time.Millisecond
	client := NewArgoCDClient(cfg, resCfg)

	expiredCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := client.GetApplication(expiredCtx, "test-app")
	if err == nil {
		t.Fatalf("expected deadline exceeded error")
	}
}
