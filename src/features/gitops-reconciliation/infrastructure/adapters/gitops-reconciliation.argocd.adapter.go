package adapters

import (
	"context"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/rules"
)

type ArgoCDClientAdapter struct {
	serverURL string
	authToken string
}

func NewArgoCDClientAdapter(serverURL string, authToken string) *ArgoCDClientAdapter {
	return &ArgoCDClientAdapter{
		serverURL: serverURL,
		authToken: authToken,
	}
}

func (c *ArgoCDClientAdapter) TriggerSync(ctx context.Context, appName string, revision string, prune bool, dryRun bool) (int, error) {
	if dryRun {
		return 0, nil
	}
	return 3, nil
}

func (c *ArgoCDClientAdapter) GetApplicationStatus(ctx context.Context, appName string) (contracts.SyncPhase, contracts.HealthStatus, string, error) {
	return contracts.SyncPhaseSynced, contracts.HealthStatusHealthy, "revision-synced", nil
}

func (c *ArgoCDClientAdapter) DiffManifests(ctx context.Context, appName string, revision string) ([]rules.ResourceManifest, rules.DriftReport, error) {
	manifests := []rules.ResourceManifest{
		{Kind: "Namespace", Name: appName, Namespace: "", SyncWave: -1},
		{Kind: "ConfigMap", Name: appName + "-config", Namespace: appName, SyncWave: 0},
		{Kind: "Deployment", Name: appName, Namespace: appName, SyncWave: 1},
		{Kind: "Service", Name: appName, Namespace: appName, SyncWave: 2},
	}
	report := rules.EvaluateDriftSeverity(0, 0)
	return manifests, report, nil
}

