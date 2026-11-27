package tests

import (
	"context"
	"fmt"
	"testing"

	"internal-developer-platform/src/features/gitops-reconciliation/internal/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/domain/rules"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/service"
)

type MockFlakyArgoClient struct {
	failRevision string
	syncCount    map[string]int
}

func NewMockFlakyArgoClient(failRevision string) *MockFlakyArgoClient {
	return &MockFlakyArgoClient{
		failRevision: failRevision,
		syncCount:    make(map[string]int),
	}
}

func (m *MockFlakyArgoClient) TriggerSync(ctx context.Context, appName string, revision string, prune bool, dryRun bool) (int, error) {
	m.syncCount[revision]++
	if revision == m.failRevision {
		return 0, fmt.Errorf("simulated deployment crash on revision %s", revision)
	}
	return 2, nil
}

func (m *MockFlakyArgoClient) GetApplicationStatus(ctx context.Context, appName string) (contracts.SyncPhase, contracts.HealthStatus, string, error) {
	return contracts.SyncPhaseSynced, contracts.HealthStatusHealthy, "active-rev", nil
}

func (m *MockFlakyArgoClient) DiffManifests(ctx context.Context, appName string, revision string) ([]rules.ResourceManifest, rules.DriftReport, error) {
	manifests := []rules.ResourceManifest{
		{Kind: "Namespace", Name: appName, SyncWave: -1},
		{Kind: "ConfigMap", Name: appName + "-cfg", SyncWave: 0},
		{Kind: "Deployment", Name: appName, SyncWave: 1},
	}
	return manifests, rules.EvaluateDriftSeverity(0, 0), nil
}

func (m *MockFlakyArgoClient) GetDetailedDrift(ctx context.Context, appName string, revision string) (*contracts.DetailedDriftReport, error) {
	return &contracts.DetailedDriftReport{
		AppName:         appName,
		TargetRevision:  revision,
		HasDrift:        false,
		Items:           make([]contracts.ClassifiedDriftItem, 0),
		TotalDriftCount: 0,
		IgnoredCount:    0,
		CanAutoSync:     true,
	}, nil
}

func (m *MockFlakyArgoClient) RefreshAppsForRepo(ctx context.Context, repoURL string, branch string) ([]string, error) {
	return []string{appNameFromURL(repoURL, branch)}, nil
}

func appNameFromURL(repoURL, branch string) string {
	return "app-" + branch
}

func TestStatefulGitOpsRollbackOnDeploymentFailure(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryGitOpsRepositoryAdapter()
	flakyClient := NewMockFlakyArgoClient("v2.1.0-corrupt")
	svc := service.NewGitOpsReconciliationService(store, flakyClient)

	seedCmd := contracts.SyncCommand{
		ID:                 "rec-seed-1",
		TenantID:           "tenant-alpha",
		ServiceID:          "order-service",
		AppName:            "order-service",
		TargetRevision:     "v2.0.0-stable",
		Prune:              true,
		DryRun:             false,
		ProjectEnforcesSig: true,
		CommitSigned:       true,
		AllowedSigningKey:  true,
		RoleReviewed:       true,
		InAllowWindow:      true,
		IdempotencyKey:     "seed-key-01",
	}

	seedRes, err := svc.TriggerReconciliation(ctx, seedCmd)
	if err != nil {
		t.Fatalf("expected seed deployment to succeed, got: %v", err)
	}
	if seedRes.Reconciliation.CurrentRevision != "v2.0.0-stable" {
		t.Fatalf("expected current revision v2.0.0-stable, got %s", seedRes.Reconciliation.CurrentRevision)
	}

	corruptCmd := contracts.SyncCommand{
		ID:                 "rec-corrupt-2",
		TenantID:           "tenant-alpha",
		ServiceID:          "order-service",
		AppName:            "order-service",
		TargetRevision:     "v2.1.0-corrupt",
		Prune:              true,
		DryRun:             false,
		ProjectEnforcesSig: true,
		CommitSigned:       true,
		AllowedSigningKey:  true,
		RoleReviewed:       true,
		InAllowWindow:      true,
		IdempotencyKey:     "corrupt-key-02",
	}

	corruptRes, err := svc.TriggerReconciliation(ctx, corruptCmd)
	if err != nil {
		t.Fatalf("unexpected error: stateful rollback should handle failure gracefully, got: %v", err)
	}

	if !corruptRes.Reconciliation.RollbackTriggered {
		t.Fatalf("expected RollbackTriggered to be true")
	}
	if corruptRes.Reconciliation.SyncPhase != contracts.SyncPhaseSynced {
		t.Fatalf("expected SyncPhase SYNCED after rollback, got %s", corruptRes.Reconciliation.SyncPhase)
	}
	if corruptRes.Reconciliation.CurrentRevision != "v2.0.0-stable" {
		t.Fatalf("expected revision to roll back to v2.0.0-stable, got %s", corruptRes.Reconciliation.CurrentRevision)
	}
	if corruptRes.Reconciliation.RollbackDetails == nil || !corruptRes.Reconciliation.RollbackDetails.Success {
		t.Fatalf("expected RollbackDetails to indicate successful rollback")
	}
}

func TestSyncWaveGroupingRule(t *testing.T) {
	manifests := []rules.ResourceManifest{
		{Kind: "Deployment", Name: "app", SyncWave: 1},
		{Kind: "Namespace", Name: "ns", SyncWave: -1},
		{Kind: "Service", Name: "svc", SyncWave: 2},
		{Kind: "ConfigMap", Name: "cfg", SyncWave: 0},
		{Kind: "Secret", Name: "sec", SyncWave: 0},
	}

	batches := rules.GroupManifestsBySyncWave(manifests)
	if len(batches) != 4 {
		t.Fatalf("expected 4 wave batches, got %d", len(batches))
	}
	if batches[0].Wave != -1 || len(batches[0].Resources) != 1 {
		t.Errorf("expected first wave to be -1 with 1 resource")
	}
	if batches[1].Wave != 0 || len(batches[1].Resources) != 2 {
		t.Errorf("expected second wave to be 0 with 2 resources")
	}
	if batches[2].Wave != 1 || len(batches[2].Resources) != 1 {
		t.Errorf("expected third wave to be 1 with 1 resource")
	}
	if batches[3].Wave != 2 || len(batches[3].Resources) != 1 {
		t.Errorf("expected fourth wave to be 2 with 1 resource")
	}
}
