package adapters

import (
	"context"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/rules"
	"internal-developer-platform/src/infra/clients/argocd"
)

type ArgoCDClientAdapter struct {
	serverURL string
	authToken string
	client    *argocd.ArgoCDClient
}

func NewArgoCDClientAdapter(serverURL string, authToken string) *ArgoCDClientAdapter {
	return &ArgoCDClientAdapter{
		serverURL: serverURL,
		authToken: authToken,
		client:    nil,
	}
}

func NewLiveArgoCDClientAdapter(client *argocd.ArgoCDClient) *ArgoCDClientAdapter {
	return &ArgoCDClientAdapter{
		client: client,
	}
}

func (c *ArgoCDClientAdapter) TriggerSync(ctx context.Context, appName string, revision string, prune bool, dryRun bool) (int, error) {
	if c.client != nil {
		resp, err := c.client.TriggerSync(ctx, appName, revision, prune, dryRun)
		if err != nil {
			return 0, err
		}
		if resp != nil && resp.Status.OperationState != nil {
			count := len(resp.Status.OperationState.SyncResult.Resources)
			if count > 0 {
				return count, nil
			}
		}
		return 1, nil
	}

	if dryRun {
		return 0, nil
	}
	return 3, nil
}

func (c *ArgoCDClientAdapter) GetApplicationStatus(ctx context.Context, appName string) (contracts.SyncPhase, contracts.HealthStatus, string, error) {
	if c.client != nil {
		resp, err := c.client.GetApplication(ctx, appName)
		if err != nil {
			return contracts.SyncPhaseFailed, contracts.HealthStatusUnknown, "", err
		}

		syncPhase := contracts.SyncPhaseSyncing
		switch resp.Status.Sync.Status {
		case argocd.SyncPhaseSynced:
			syncPhase = contracts.SyncPhaseSynced
		case argocd.SyncPhaseOutOfSync:
			syncPhase = contracts.SyncPhaseDriftDetected
		default:
			syncPhase = contracts.SyncPhasePending
		}

		healthStatus := contracts.HealthStatusUnknown
		switch resp.Status.Health.Status {
		case argocd.HealthStatusHealthy:
			healthStatus = contracts.HealthStatusHealthy
		case argocd.HealthStatusProgressing:
			healthStatus = contracts.HealthStatusProgressing
		case argocd.HealthStatusDegraded:
			healthStatus = contracts.HealthStatusDegraded
		case argocd.HealthStatusMissing:
			healthStatus = contracts.HealthStatusMissing
		}

		return syncPhase, healthStatus, resp.Status.Sync.Revision, nil
	}

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
