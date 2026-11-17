package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gitopsreconciliation "internal-developer-platform/src/features/gitops-reconciliation"
	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/infra/clients/argocd"
	"internal-developer-platform/src/infra/clients/resilience"
)

func TestGitOpsReconciliationServiceExecution(t *testing.T) {
	ctx := context.Background()
	store := gitopsreconciliation.NewMemoryStore()
	argoClient := gitopsreconciliation.NewArgoCDClient("https://argocd.internal:443", "test-token")
	svc := gitopsreconciliation.NewService(store, argoClient)

	cmd := contracts.SyncCommand{
		ID:             "rec-test-1",
		TenantID:       "tenant-dev",
		ServiceID:      "payment-service",
		AppName:        "payment-service-dev",
		TargetRevision: "v1.2.0",
		Prune:          true,
		DryRun:         false,
		IdempotencyKey: "idemp-001",
	}

	result, err := svc.TriggerReconciliation(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error on sync: %v", err)
	}

	if result.Reconciliation.SyncPhase != contracts.SyncPhaseSynced {
		t.Errorf("expected phase SYNCED, got %s", result.Reconciliation.SyncPhase)
	}
	if result.Reconciliation.HealthStatus != contracts.HealthStatusHealthy {
		t.Errorf("expected health HEALTHY, got %s", result.Reconciliation.HealthStatus)
	}
	if result.Reconciliation.CurrentRevision != "v1.2.0" {
		t.Errorf("expected revision v1.2.0, got %s", result.Reconciliation.CurrentRevision)
	}

	replay, err := svc.TriggerReconciliation(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error on idempotent replay: %v", err)
	}
	if replay.Message != "idempotent replay" {
		t.Errorf("expected idempotent replay message, got: %s", replay.Message)
	}

	fetched, err := svc.GetReconciliation(ctx, "tenant-dev", "rec-test-1")
	if err != nil {
		t.Fatalf("unexpected error fetching record: %v", err)
	}
	if fetched.ID != "rec-test-1" {
		t.Errorf("expected ID rec-test-1, got %s", fetched.ID)
	}

	list, err := svc.ListReconciliations(ctx, contracts.FilterCriteria{TenantID: "tenant-dev"})
	if err != nil {
		t.Fatalf("unexpected error listing records: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 record in list, got %d", len(list))
	}
}

func TestGitOpsReconciliationWithLiveArgoCDClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"metadata": {"name": "billing-app", "namespace": "argocd"},
			"status": {
				"sync": {"status": "Synced", "revision": "v2.0.0"},
				"health": {"status": "Healthy", "message": "all pods healthy"},
				"operationState": {
					"phase": "Succeeded",
					"syncResult": {
						"resources": [
							{"kind": "Deployment", "name": "billing-app", "status": "Synced", "syncWave": 0},
							{"kind": "Service", "name": "billing-app", "status": "Synced", "syncWave": 1}
						]
					}
				}
			}
		}`))
	}))
	defer server.Close()

	argoCfg := argocd.ArgoCDClientConfig{
		BaseURL:   server.URL,
		AuthToken: "live-token",
		Timeout:   5 * time.Second,
	}
	resCfg := resilience.DefaultResilienceConfig()
	liveClient := argocd.NewArgoCDClient(argoCfg, resCfg)

	store := gitopsreconciliation.NewMemoryStore()
	adapter := gitopsreconciliation.NewLiveArgoCDClient(liveClient)
	svc := gitopsreconciliation.NewService(store, adapter)

	cmd := contracts.SyncCommand{
		ID:             "rec-live-1",
		TenantID:       "tenant-billing",
		ServiceID:      "billing-app",
		AppName:        "billing-app",
		TargetRevision: "v2.0.0",
		Prune:          true,
		DryRun:         false,
		IdempotencyKey: "idemp-live-001",
	}

	result, err := svc.TriggerReconciliation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("unexpected error on live sync: %v", err)
	}

	if result.Reconciliation.SyncPhase != contracts.SyncPhaseSynced {
		t.Fatalf("expected Synced, got %s", result.Reconciliation.SyncPhase)
	}
	if result.Reconciliation.HealthStatus != contracts.HealthStatusHealthy {
		t.Fatalf("expected Healthy, got %s", result.Reconciliation.HealthStatus)
	}
	if result.AppliedHooks != 2 {
		t.Fatalf("expected 2 applied hooks/resources, got %d", result.AppliedHooks)
	}
}

