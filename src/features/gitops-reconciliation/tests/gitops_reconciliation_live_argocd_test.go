package tests

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	gitopsreconciliation "internal-developer-platform/src/features/gitops-reconciliation"
	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/infra/clients/argocd"
	"internal-developer-platform/src/infra/clients/resilience"
)

func TestLiveArgoCDClusterReconciliation(t *testing.T) {
	baseURL := os.Getenv("ARGOCD_SERVER_URL")
	if baseURL == "" {
		baseURL = "https://127.0.0.1:8085"
	}
	token := os.Getenv("ARGOCD_TOKEN")
	if token == "" {
		t.Skip("skipping live ArgoCD cluster test: ARGOCD_TOKEN not set")
	}
	argoCfg := argocd.ArgoCDClientConfig{
		BaseURL:            baseURL,
		AuthToken:          token,
		InsecureSkipVerify: true,
		Timeout:            10 * time.Second,
	}

	resCfg := resilience.DefaultResilienceConfig()
	client := argocd.NewArgoCDClient(argoCfg, resCfg)

	adapter := gitopsreconciliation.NewLiveArgoCDClient(client)

	ctx := context.Background()
	appName := "internal-developer-platform-dev"

	phase, health, revision, err := adapter.GetApplicationStatus(ctx, appName)
	if err != nil {
		t.Skipf("skipping live ArgoCD cluster test, cluster unreachable: %v", err)
	}

	t.Logf("Live ArgoCD Application Status -> Phase: %s, Health: %s, Revision: %s", phase, health, revision)

	if phase == contracts.SyncPhaseFailed {
		t.Errorf("expected application to not be in FAILED phase initially")
	}

	db := getTestPostgresDB(t)
	defer db.Close()

	pgStore := gitopsreconciliation.NewPostgresStore(db)
	svc := gitopsreconciliation.NewService(pgStore, adapter)

	cmd := contracts.SyncCommand{
		ID:             fmt.Sprintf("rec-live-cluster-%d", time.Now().UnixNano()),
		TenantID:       "platform-tenant",
		ServiceID:      "internal-developer-platform",
		AppName:        appName,
		TargetRevision: "HEAD",
		Prune:          true,
		DryRun:         true,
		IdempotencyKey: fmt.Sprintf("idemp-cluster-%d", time.Now().UnixNano()),
	}

	result, err := svc.TriggerReconciliation(ctx, cmd)
	if err != nil {
		t.Fatalf("failed to execute reconciliation against real ArgoCD server: %v", err)
	}

	t.Logf("Reconciliation Result -> Phase: %s, Health: %s, Message: %s",
		result.Reconciliation.SyncPhase,
		result.Reconciliation.HealthStatus,
		result.Message,
	)

	if result.Reconciliation.SyncPhase != contracts.SyncPhaseSynced {
		t.Errorf("expected SyncPhase SYNCED on dry-run against real ArgoCD, got %s", result.Reconciliation.SyncPhase)
	}

	persisted, err := pgStore.GetByID(ctx, cmd.TenantID, cmd.ID)
	if err != nil {
		t.Fatalf("failed to verify record in Postgres: %v", err)
	}
	if persisted.ID != cmd.ID {
		t.Errorf("expected persisted ID %s, got %s", cmd.ID, persisted.ID)
	}
}
