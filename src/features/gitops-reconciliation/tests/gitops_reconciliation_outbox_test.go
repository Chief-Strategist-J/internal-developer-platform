package tests

import (
	"context"
	"testing"

	gitopsreconciliation "internal-developer-platform/src/features/gitops-reconciliation"
	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
)

func TestGitOpsReconciliationOutboxEventEmission(t *testing.T) {
	ctx := context.Background()
	store := gitopsreconciliation.NewMemoryStore()
	argoClient := gitopsreconciliation.NewArgoCDClient("https://argocd.internal:443", "test-token")
	outbox := gitopsreconciliation.NewMemoryOutboxAdapter()

	svc := gitopsreconciliation.NewService(store, argoClient)
	svc.SetOutboxPort(outbox)

	cmd := contracts.SyncCommand{
		ID:                 "rec-outbox-01",
		TenantID:           "tenant-outbox",
		ServiceID:          "svc-outbox",
		AppName:            "outbox-app",
		TargetRevision:     "v1.0.0",
		ProjectEnforcesSig: true,
		CommitSigned:       true,
		AllowedSigningKey:  true,
		RoleReviewed:       true,
		InAllowWindow:      true,
		IdempotencyKey:     "idem-outbox-01",
	}

	res, err := svc.TriggerReconciliation(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error on sync: %v", err)
	}
	if res.Reconciliation.SyncPhase != contracts.SyncPhaseSynced {
		t.Fatalf("expected SYNCED, got %s", res.Reconciliation.SyncPhase)
	}
	if len(outbox.Events) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(outbox.Events))
	}
	if outbox.Events[0].EventType != contracts.OutboxEventReconciliationSucceeded {
		t.Errorf("expected ReconciliationSucceeded, got %s", outbox.Events[0].EventType)
	}
	if outbox.Events[0].AggregateID != "rec-outbox-01" {
		t.Errorf("expected aggregateId rec-outbox-01, got %s", outbox.Events[0].AggregateID)
	}

	relayedCount := 0
	relayed, err := svc.RelayPendingOutboxEvents(ctx, 10, func(ctx context.Context, ev contracts.OutboxEvent) error {
		relayedCount++
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error relaying outbox events: %v", err)
	}
	if relayed != 1 || relayedCount != 1 {
		t.Fatalf("expected 1 relayed event, got %d (callback %d)", relayed, relayedCount)
	}

	remaining, err := outbox.FetchUnpublished(ctx, 10)
	if err != nil {
		t.Fatalf("unexpected error fetching unpublished: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected 0 remaining unpublished events, got %d", len(remaining))
	}
}

func TestBreakGlassAutoSyncGuard(t *testing.T) {
	ctx := context.Background()
	store := gitopsreconciliation.NewMemoryStore()
	argoClient := gitopsreconciliation.NewArgoCDClient("https://argocd.internal:443", "test-token")
	svc := gitopsreconciliation.NewService(store, argoClient)

	cmd := contracts.SyncCommand{
		ID:                          "rec-bg-01",
		TenantID:                    "tenant-bg",
		ServiceID:                   "svc-bg",
		AppName:                     "bg-app",
		TargetRevision:              "v1.0.0",
		ProjectEnforcesSig:          true,
		CommitSigned:                true,
		AllowedSigningKey:           true,
		RoleReviewed:                true,
		InAllowWindow:               true,
		EmergencyChangePending:      true,
		EmergencyCommittedAndSigned: false,
		IdempotencyKey:              "idem-bg-01",
	}

	_, err := svc.TriggerReconciliation(ctx, cmd)
	if err == nil {
		t.Fatal("expected break-glass guard to block sync when emergency change is uncommitted")
	}
}
