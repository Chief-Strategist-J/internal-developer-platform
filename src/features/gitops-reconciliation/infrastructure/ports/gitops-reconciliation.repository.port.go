package ports

import (
	"context"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/rules"
)

type GitOpsStorePort interface {
	Save(ctx context.Context, record contracts.ReconciliationRecord) error
	GetByID(ctx context.Context, tenantID string, id string) (*contracts.ReconciliationRecord, error)
	GetByIdempotencyKey(ctx context.Context, tenantID string, key string) (*contracts.ReconciliationRecord, error)
	List(ctx context.Context, filter contracts.FilterCriteria) ([]contracts.ReconciliationRecord, error)
	Update(ctx context.Context, record contracts.ReconciliationRecord) error
}

type ArgoCDClientPort interface {
	TriggerSync(ctx context.Context, appName string, revision string, prune bool, dryRun bool) (int, error)
	GetApplicationStatus(ctx context.Context, appName string) (contracts.SyncPhase, contracts.HealthStatus, string, error)
	DiffManifests(ctx context.Context, appName string, revision string) ([]rules.ResourceManifest, rules.DriftReport, error)
}

