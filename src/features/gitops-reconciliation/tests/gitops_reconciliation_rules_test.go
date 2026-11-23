package tests

import (
	"testing"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/rules"
)

func TestSortBySyncWave(t *testing.T) {
	manifests := []rules.ResourceManifest{
		{Kind: "Service", Name: "app-svc", SyncWave: 2},
		{Kind: "Namespace", Name: "app-ns", SyncWave: -1},
		{Kind: "Deployment", Name: "app-deploy", SyncWave: 1},
		{Kind: "ConfigMap", Name: "app-cfg", SyncWave: 0},
	}

	sorted := rules.SortBySyncWave(manifests)
	if len(sorted) != 4 {
		t.Fatalf("expected 4 manifests, got %d", len(sorted))
	}
	if sorted[0].Kind != "Namespace" || sorted[0].SyncWave != -1 {
		t.Errorf("expected first manifest to be Namespace (-1), got %s (%d)", sorted[0].Kind, sorted[0].SyncWave)
	}
	if sorted[3].Kind != "Service" || sorted[3].SyncWave != 2 {
		t.Errorf("expected last manifest to be Service (2), got %s (%d)", sorted[3].Kind, sorted[3].SyncWave)
	}
}

func TestEvaluateDriftSeverity(t *testing.T) {
	clean := rules.EvaluateDriftSeverity(0, 0)
	if clean.HasDrift || clean.Critical {
		t.Errorf("expected clean report, got hasDrift=%v critical=%v", clean.HasDrift, clean.Critical)
	}

	minor := rules.EvaluateDriftSeverity(2, 0)
	if !minor.HasDrift || minor.Critical {
		t.Errorf("expected non-critical drift, got hasDrift=%v critical=%v", minor.HasDrift, minor.Critical)
	}

	criticalDelete := rules.EvaluateDriftSeverity(0, 1)
	if !criticalDelete.Critical {
		t.Errorf("expected critical drift on deleted resource")
	}

	criticalModified := rules.EvaluateDriftSeverity(6, 0)
	if !criticalModified.Critical {
		t.Errorf("expected critical drift when modified > 5")
	}
}

func TestCanInitiateSync(t *testing.T) {
	if !rules.CanInitiateSync(contracts.SyncPhasePending) {
		t.Errorf("expected sync allowed on pending")
	}
	if !rules.CanInitiateSync(contracts.SyncPhaseSynced) {
		t.Errorf("expected sync allowed on synced")
	}
	if rules.CanInitiateSync(contracts.SyncPhaseSyncing) {
		t.Errorf("expected sync blocked while currently syncing")
	}
}
