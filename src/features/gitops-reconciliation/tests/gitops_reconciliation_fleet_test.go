package tests

import (
	"context"
	"testing"

	"internal-developer-platform/src/features/gitops-reconciliation/internal/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/service"
)

func TestFleetRingRolloutSuccess(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryGitOpsRepositoryAdapter()
	argoClient := adapters.NewArgoCDClientAdapter("https://argocd.internal:443", "test-token")
	svc := service.NewGitOpsReconciliationService(store, argoClient)

	cmd := contracts.FleetRolloutCommand{
		TenantID:       "tenant-fleet",
		AppName:        "checkout",
		TargetRevision: "v2.0.0",
		Targets: []contracts.FleetRolloutTarget{
			{ClusterName: "canary-1", Ring: contracts.ClusterRingCanary, Namespace: "prod", ProjectName: "checkout"},
			{ClusterName: "prod-1", Ring: contracts.ClusterRingProdWave1, Namespace: "prod", ProjectName: "checkout"},
		},
	}

	res, err := svc.ExecuteFleetRingRollout(ctx, cmd)
	if err != nil {
		t.Fatalf("expected fleet rollout to succeed, got %v", err)
	}
	if res.Halted {
		t.Fatalf("expected Halted=false, got true at ring %s", res.HaltedRing)
	}
	if len(res.Results) != 2 {
		t.Fatalf("expected 2 target results, got %d", len(res.Results))
	}
}
