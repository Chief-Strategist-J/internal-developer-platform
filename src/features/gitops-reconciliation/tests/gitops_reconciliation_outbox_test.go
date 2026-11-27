package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

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

func TestOutboxRelayMultiSpanTracing(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	}()

	ctx := context.Background()
	store := gitopsreconciliation.NewMemoryStore()
	argoClient := gitopsreconciliation.NewArgoCDClient("https://argocd.internal:443", "test-token")
	outbox := gitopsreconciliation.NewMemoryOutboxAdapter()
	svc := gitopsreconciliation.NewService(store, argoClient)
	svc.SetOutboxPort(outbox)

	if err := outbox.Enqueue(ctx, contracts.OutboxEvent{
		ID:          "evt-trace-01",
		TenantID:    "tenant-trace",
		AggregateID: "rec-trace-01",
		EventType:   contracts.OutboxEventReconciliationSucceeded,
		AppName:     "trace-app",
		Revision:    "v1.2.3",
		OccurredAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("unexpected enqueue error: %v", err)
	}
	exporter.Reset()

	relayed, err := svc.RelayPendingOutboxEvents(ctx, 10, func(ctx context.Context, ev contracts.OutboxEvent) error {
		return nil
	})
	if err != nil || relayed != 1 {
		t.Fatalf("expected 1 relayed event, got %d (err=%v)", relayed, err)
	}

	spans := exporter.GetSpans()
	expected := map[string]bool{
		"gitops.reconciliation.outbox_relay":               false,
		"gitops.reconciliation.outbox_relay.fetch":         false,
		"gitops.outbox.memory.fetch_unpublished":           false,
		"gitops.reconciliation.outbox_relay.process_event": false,
		"gitops.reconciliation.outbox_relay.publish":       false,
		"gitops.outbox.memory.mark_published":              false,
	}
	parentByName := make(map[string]string, len(spans))
	idToName := make(map[string]string, len(spans))
	for _, s := range spans {
		idToName[s.SpanContext.SpanID().String()] = s.Name
		if _, ok := expected[s.Name]; ok {
			expected[s.Name] = true
		}
		if s.Parent.IsValid() {
			parentByName[s.Name] = s.Parent.SpanID().String()
		}
	}
	for name, seen := range expected {
		if !seen {
			t.Errorf("expected span %s to be emitted", name)
		}
	}
	if idToName[parentByName["gitops.reconciliation.outbox_relay.publish"]] != "gitops.reconciliation.outbox_relay.process_event" {
		t.Errorf("expected publish span parent to be process_event")
	}

	exporter.Reset()
	if err := outbox.Enqueue(ctx, contracts.OutboxEvent{
		ID:          "evt-trace-err",
		TenantID:    "tenant-trace",
		AggregateID: "rec-trace-02",
		EventType:   contracts.OutboxEventReconciliationFailed,
		AppName:     "trace-app",
		Revision:    "v1.2.4",
		OccurredAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("unexpected enqueue error: %v", err)
	}
	exporter.Reset()

	_, relayErr := svc.RelayPendingOutboxEvents(ctx, 10, func(ctx context.Context, ev contracts.OutboxEvent) error {
		return errors.New("broker unavailable")
	})
	if relayErr == nil {
		t.Fatal("expected publish error to be returned")
	}

	errSpans := exporter.GetSpans()
	errorMarked := map[string]bool{
		"gitops.reconciliation.outbox_relay":               false,
		"gitops.reconciliation.outbox_relay.process_event": false,
		"gitops.reconciliation.outbox_relay.publish":       false,
	}
	for _, s := range errSpans {
		if _, tracked := errorMarked[s.Name]; tracked && s.Status.Code == codes.Error {
			errorMarked[s.Name] = true
		}
	}
	for name, ok := range errorMarked {
		if !ok {
			t.Errorf("expected span %s to be marked with codes.Error", name)
		}
	}
}
