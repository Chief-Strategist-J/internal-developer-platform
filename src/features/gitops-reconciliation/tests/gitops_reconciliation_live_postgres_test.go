package tests

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"internal-developer-platform/src/features/gitops-reconciliation/internal/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/service"
	"internal-developer-platform/src/infra/observability"
)

func getTestPostgresDB(t *testing.T) *sql.DB {
	dbURL := "postgres://postgres:postgres_dev_password_123@127.0.0.1:5432/idp_db?sslmode=disable"
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Skipf("skipping live postgres test: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("skipping live postgres test, db unreachable: %v", err)
	}
	return db
}

func TestLivePostgresGitOpsReconciliationEndToEnd(t *testing.T) {
	db := getTestPostgresDB(t)
	defer db.Close()

	tp, shutdown, err := observability.InitTracer("gitops-test-suite")
	if err == nil && tp != nil {
		defer func() { _ = shutdown(context.Background()) }()
	}

	pgStore := adapters.NewPostgresGitOpsRepositoryAdapter(db)
	flakyArgo := NewMockFlakyArgoClient("v3.0.0-bad")
	svc := service.NewGitOpsReconciliationService(pgStore, flakyArgo)

	tenantID := fmt.Sprintf("tenant-e2e-%d", time.Now().UnixNano())
	serviceID := "auth-gateway"
	appName := "auth-gateway-prod"

	ctx := context.Background()

	seedCmd := contracts.SyncCommand{
		ID:                 fmt.Sprintf("rec-live-seed-%d", time.Now().UnixNano()),
		TenantID:           tenantID,
		ServiceID:          serviceID,
		AppName:            appName,
		TargetRevision:     "v2.5.0-stable",
		Prune:              true,
		DryRun:             false,
		ProjectEnforcesSig: true,
		CommitSigned:       true,
		AllowedSigningKey:  true,
		RoleReviewed:       true,
		InAllowWindow:      true,
		IdempotencyKey:     fmt.Sprintf("idemp-live-seed-%d", time.Now().UnixNano()),
	}

	res1, err := svc.TriggerReconciliation(ctx, seedCmd)
	if err != nil {
		t.Fatalf("expected live seed sync to succeed, got: %v", err)
	}
	if res1.Reconciliation.SyncPhase != contracts.SyncPhaseSynced {
		t.Errorf("expected phase SYNCED, got %s", res1.Reconciliation.SyncPhase)
	}
	if res1.Reconciliation.CurrentRevision != "v2.5.0-stable" {
		t.Errorf("expected revision v2.5.0-stable, got %s", res1.Reconciliation.CurrentRevision)
	}

	persistedSeed, err := pgStore.GetByID(ctx, tenantID, seedCmd.ID)
	if err != nil {
		t.Fatalf("failed to retrieve seed record from postgres: %v", err)
	}
	if persistedSeed.CurrentRevision != "v2.5.0-stable" {
		t.Errorf("expected persisted current revision v2.5.0-stable, got %s", persistedSeed.CurrentRevision)
	}

	corruptCmd := contracts.SyncCommand{
		ID:                 fmt.Sprintf("rec-live-corrupt-%d", time.Now().UnixNano()),
		TenantID:           tenantID,
		ServiceID:          serviceID,
		AppName:            appName,
		TargetRevision:     "v3.0.0-bad",
		Prune:              true,
		DryRun:             false,
		ProjectEnforcesSig: true,
		CommitSigned:       true,
		AllowedSigningKey:  true,
		RoleReviewed:       true,
		InAllowWindow:      true,
		IdempotencyKey:     fmt.Sprintf("idemp-live-corrupt-%d", time.Now().UnixNano()),
	}

	res2, err := svc.TriggerReconciliation(ctx, corruptCmd)
	if err != nil {
		t.Fatalf("expected rollback to handle error gracefully, got: %v", err)
	}
	if !res2.Reconciliation.RollbackTriggered {
		t.Errorf("expected RollbackTriggered to be true")
	}
	if res2.Reconciliation.CurrentRevision != "v2.5.0-stable" {
		t.Errorf("expected rollback to restore v2.5.0-stable, got %s", res2.Reconciliation.CurrentRevision)
	}
	if res2.Reconciliation.SyncPhase != contracts.SyncPhaseSynced {
		t.Errorf("expected phase SYNCED after rollback, got %s", res2.Reconciliation.SyncPhase)
	}

	persistedCorrupt, err := pgStore.GetByID(ctx, tenantID, corruptCmd.ID)
	if err != nil {
		t.Fatalf("failed to retrieve corrupt record from postgres: %v", err)
	}
	if persistedCorrupt.CurrentRevision != "v2.5.0-stable" {
		t.Errorf("expected persisted rollback to match v2.5.0-stable, got %s", persistedCorrupt.CurrentRevision)
	}
}

func TestLivePostgresConcurrentIdempotencyGate(t *testing.T) {
	db := getTestPostgresDB(t)
	defer db.Close()

	pgStore := adapters.NewPostgresGitOpsRepositoryAdapter(db)
	argo := NewMockFlakyArgoClient("")
	svc := service.NewGitOpsReconciliationService(pgStore, argo)

	sharedKey := fmt.Sprintf("concurrent-key-%d", time.Now().UnixNano())
	tenantID := fmt.Sprintf("tenant-concurrency-%d", time.Now().UnixNano())

	concurrency := 10
	var wg sync.WaitGroup
	wg.Add(concurrency)

	type workerResult struct {
		res *contracts.SyncResult
		err error
	}
	results := make([]workerResult, concurrency)

	for i := 0; i < concurrency; i++ {
		idx := i
		go func() {
			defer wg.Done()
			cmd := contracts.SyncCommand{
				ID:                 fmt.Sprintf("rec-conc-%d-%d", idx, time.Now().UnixNano()),
				TenantID:           tenantID,
				ServiceID:          "order-service",
				AppName:            "order-service",
				TargetRevision:     "v1.0.0",
				Prune:              true,
				DryRun:             false,
				ProjectEnforcesSig: true,
				CommitSigned:       true,
				AllowedSigningKey:  true,
				RoleReviewed:       true,
				InAllowWindow:      true,
				IdempotencyKey:     sharedKey,
			}
			res, err := svc.TriggerReconciliation(context.Background(), cmd)
			results[idx] = workerResult{res: res, err: err}
		}()
	}

	wg.Wait()

	var successCount int
	var conflictCount int

	for _, r := range results {
		if r.err == nil && r.res != nil {
			successCount++
		} else if r.err != nil {
			conflictCount++
		}
	}

	if successCount == 0 {
		t.Fatalf("expected at least one request to succeed, got 0")
	}

	var rowCount int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM gitops_reconciliations WHERE tenant_id = $1 AND idempotency_key = $2",
		tenantID, sharedKey,
	).Scan(&rowCount)
	if err != nil {
		t.Fatalf("failed to query row count: %v", err)
	}
	if rowCount != 1 {
		t.Fatalf("expected exactly 1 row in postgres, got %d (idempotency violation!)", rowCount)
	}
}
