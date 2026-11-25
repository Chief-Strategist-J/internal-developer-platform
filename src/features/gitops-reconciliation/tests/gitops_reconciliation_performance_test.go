package tests

import (
	"context"
	"testing"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/machines"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/rules"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/workflows"
)

func BenchmarkSortBySyncWave(b *testing.B) {
	manifests := []rules.ResourceManifest{
		{Kind: "Service", Name: "svc", Namespace: "dev", SyncWave: 2},
		{Kind: "Namespace", Name: "dev", Namespace: "", SyncWave: -1},
		{Kind: "Deployment", Name: "app", Namespace: "dev", SyncWave: 1},
		{Kind: "ConfigMap", Name: "cfg", Namespace: "dev", SyncWave: 0},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = rules.SortBySyncWave(manifests)
	}
}

func BenchmarkTransitionSyncState(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = machines.TransitionSyncState(contracts.SyncPhasePending, machines.EventStartSync)
	}
}

func BenchmarkValidateSyncCommand(b *testing.B) {
	cmd := contracts.SyncCommand{
		ID:             "rec-bench-001",
		TenantID:       "tenant-bench",
		ServiceID:      "svc-bench",
		AppName:        "bench-app",
		TargetRevision: "main",
		IdempotencyKey: "idem-bench-001",
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = contracts.ValidateSyncCommand(cmd)
	}
}

func BenchmarkEvaluateDriftSeverity(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = rules.EvaluateDriftSeverity(3, 1)
	}
}

func BenchmarkExecuteReconciliationPlan(b *testing.B) {
	ctx := context.Background()
	manifests := []rules.ResourceManifest{
		{Kind: "Namespace", Name: "dev", Namespace: "", SyncWave: -1},
		{Kind: "ConfigMap", Name: "cfg", Namespace: "dev", SyncWave: 0},
		{Kind: "Deployment", Name: "app", Namespace: "dev", SyncWave: 1},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = workflows.ExecuteReconciliationPlan(ctx, manifests)
	}
}
