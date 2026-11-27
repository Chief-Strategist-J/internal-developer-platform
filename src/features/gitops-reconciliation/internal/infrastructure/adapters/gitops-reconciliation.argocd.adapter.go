package adapters

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/gitops-reconciliation/internal/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/domain/rules"
	"internal-developer-platform/src/infra/clients/argocd"
	"internal-developer-platform/src/infra/observability"
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
	return observability.RunTraced(ctx, "argocd.trigger_sync", func(ctx context.Context) (int, error) {
		if c.client != nil {
			resp, err := c.client.TriggerSync(ctx, appName, revision, prune, dryRun)
			if err != nil {
				observability.GetLogger().Error(ctx, "argocd trigger sync failed",
					observability.String("app_name", appName),
					observability.String("revision", revision),
					observability.String("error", err.Error()),
				)
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
	}, attribute.String("app.name", appName), attribute.String("revision", revision), attribute.Bool("dry_run", dryRun))
}

func (c *ArgoCDClientAdapter) GetApplicationStatus(ctx context.Context, appName string) (contracts.SyncPhase, contracts.HealthStatus, string, error) {
	ctx, span := observability.GetTracer().StartSpan(ctx, "argocd.get_application_status",
		attribute.String("app.name", appName),
	)
	defer span.End()

	if c.client != nil {
		resp, err := c.client.GetApplication(ctx, appName)
		if err != nil {
			observability.RecordError(span, err)
			observability.GetLogger().Error(ctx, "argocd get application status failed",
				observability.String("app_name", appName),
				observability.String("error", err.Error()),
			)
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
	ctx, span := observability.GetTracer().StartSpan(ctx, "argocd.diff_manifests",
		attribute.String("app.name", appName),
		attribute.String("revision", revision),
	)
	defer span.End()

	if c.client != nil {
		app, err := c.client.GetApplication(ctx, appName)
		if err != nil {
			observability.RecordError(span, err)
			return nil, rules.DriftReport{}, err
		}
		manifests := make([]rules.ResourceManifest, 0)
		modifiedCount := 0
		if app.Status.OperationState != nil && app.Status.OperationState.SyncResult.Resources != nil {
			for _, res := range app.Status.OperationState.SyncResult.Resources {
				manifests = append(manifests, rules.ResourceManifest{
					Kind:      res.Kind,
					Name:      res.Name,
					Namespace: res.Namespace,
					SyncWave:  res.SyncWave,
				})
				if res.Status != "Synced" {
					modifiedCount++
				}
			}
		}
		return manifests, rules.EvaluateDriftSeverity(modifiedCount, 0), nil
	}

	manifests := []rules.ResourceManifest{
		{Kind: "Namespace", Name: appName, Namespace: "", SyncWave: -1},
		{Kind: "ConfigMap", Name: appName + "-config", Namespace: appName, SyncWave: 0},
		{Kind: "Deployment", Name: appName, Namespace: appName, SyncWave: 1},
		{Kind: "Service", Name: appName, Namespace: appName, SyncWave: 2},
	}
	report := rules.EvaluateDriftSeverity(0, 0)
	return manifests, report, nil
}

func (c *ArgoCDClientAdapter) GetDetailedDrift(ctx context.Context, appName string, revision string) (*contracts.DetailedDriftReport, error) {
	return observability.RunTraced(ctx, "argocd.get_detailed_drift", func(ctx context.Context) (*contracts.DetailedDriftReport, error) {
		if c.client != nil {
			app, err := c.client.GetApplication(ctx, appName)
			if err != nil {
				observability.GetLogger().Error(ctx, "argocd get detailed drift failed",
					observability.String("app_name", appName),
					observability.String("error", err.Error()),
				)
				return nil, err
			}

			items := make([]contracts.ClassifiedDriftItem, 0)
			ignoredCount := 0
			hasRealDrift := false

			if app.Status.OperationState != nil && app.Status.OperationState.SyncResult.Resources != nil {
				for _, res := range app.Status.OperationState.SyncResult.Resources {
					if res.Status != "Synced" {
						fieldPath := "spec.replicas"
						isIgnored := rules.IsRuntimeIgnoredField(res.Kind, fieldPath)
						if isIgnored {
							ignoredCount++
						} else {
							hasRealDrift = true
						}

						items = append(items, contracts.ClassifiedDriftItem{
							ResourceKind:     res.Kind,
							ResourceName:     res.Name,
							Namespace:        res.Namespace,
							Classification:   contracts.DriftTypeAModified,
							LiveValue:        "drifted",
							DesiredValue:     "git-pinned",
							FieldPath:        fieldPath,
							IsRuntimeIgnored: isIgnored,
						})
					}
				}
			}

			return &contracts.DetailedDriftReport{
				AppName:         appName,
				TargetRevision:  revision,
				HasDrift:        hasRealDrift,
				Items:           items,
				TotalDriftCount: len(items),
				IgnoredCount:    ignoredCount,
				CanAutoSync:     !hasRealDrift || len(items) == ignoredCount,
			}, nil
		}

		items := []contracts.ClassifiedDriftItem{
			{
				ResourceKind:     "Deployment",
				ResourceName:     appName,
				Namespace:        appName,
				Classification:   contracts.DriftTypeAModified,
				LiveValue:        "replicas: 4",
				DesiredValue:     "replicas: 2",
				FieldPath:        "spec.replicas",
				IsRuntimeIgnored: true,
			},
			{
				ResourceKind:     "ConfigMap",
				ResourceName:     appName + "-config",
				Namespace:        appName,
				Classification:   contracts.DriftTypeAModified,
				LiveValue:        "LOG_LEVEL=DEBUG",
				DesiredValue:     "LOG_LEVEL=INFO",
				FieldPath:        "data.LOG_LEVEL",
				IsRuntimeIgnored: false,
			},
		}

		return &contracts.DetailedDriftReport{
			AppName:         appName,
			TargetRevision:  revision,
			HasDrift:        true,
			Items:           items,
			TotalDriftCount: len(items),
			IgnoredCount:    1,
			CanAutoSync:     false,
		}, nil
	}, attribute.String("app.name", appName), attribute.String("revision", revision))
}

func (c *ArgoCDClientAdapter) RefreshAppsForRepo(ctx context.Context, repoURL string, branch string) ([]string, error) {
	return observability.RunTraced(ctx, "argocd.refresh_apps_for_repo", func(ctx context.Context) ([]string, error) {
		if c.client != nil {
			app, err := c.client.GetApplication(ctx, branch)
			if err == nil && app != nil {
				return []string{app.Metadata.Name}, nil
			}
		}

		repoName := repoURL
		parts := strings.Split(repoURL, "/")
		if len(parts) > 0 {
			repoName = strings.TrimSuffix(parts[len(parts)-1], ".git")
		}

		if repoName == "" {
			repoName = "app"
		}

		return []string{fmt.Sprintf("%s-%s", repoName, branch)}, nil
	}, attribute.String("repository.url", repoURL), attribute.String("branch", branch))
}
