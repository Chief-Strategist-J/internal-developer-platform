// Package ports declares abstract outbound interfaces for Disaster Recovery.
//
// Architectural Blueprint:
// - Step 10 Ports: Abstract boundaries decoupling domain services from concrete infrastructure.
// - Supports multiple implementations via adapters (Cloud SQL / AlloyDB / Spanner, Route53 / Cloud DNS, etc.).
// - Zero inline comments inside method bodies.
package ports

import (
	"context"

	"internal-developer-platform/src/features/disaster-recovery/contracts"
)

type DRPlanStorePort interface {
	SavePlan(ctx context.Context, plan contracts.DRPlanRecord) error
	GetPlanByID(ctx context.Context, tenantID string, id string) (*contracts.DRPlanRecord, error)
	GetPlanByIdempotencyKey(ctx context.Context, tenantID string, key string) (*contracts.DRPlanRecord, error)
	ListPlans(ctx context.Context, tenantID string) ([]contracts.DRPlanRecord, error)
	UpdatePlan(ctx context.Context, plan contracts.DRPlanRecord) error

	SaveIncident(ctx context.Context, inc contracts.DRIncidentRecord) error
	GetIncidentByID(ctx context.Context, tenantID string, id string) (*contracts.DRIncidentRecord, error)
	UpdateIncident(ctx context.Context, inc contracts.DRIncidentRecord) error

	SaveFailoverOp(ctx context.Context, op contracts.DRFailoverOperationRecord) error
	GetFailoverOpByID(ctx context.Context, tenantID string, id string) (*contracts.DRFailoverOperationRecord, error)
	UpdateFailoverOp(ctx context.Context, op contracts.DRFailoverOperationRecord) error

	SaveFailbackOp(ctx context.Context, op contracts.DRFailbackOperationRecord) error
	GetFailbackOpByID(ctx context.Context, tenantID string, id string) (*contracts.DRFailbackOperationRecord, error)
	UpdateFailbackOp(ctx context.Context, op contracts.DRFailbackOperationRecord) error

	SaveFencingEpoch(ctx context.Context, epoch contracts.FencingEpochRecord) error
	GetFencingEpoch(ctx context.Context, datasetID string) (*contracts.FencingEpochRecord, error)
	IncrementFencingEpoch(ctx context.Context, datasetID string, authority string) (*contracts.FencingEpochRecord, error)

	SaveDrillRun(ctx context.Context, drill contracts.DrillRunRecord) error
	GetDrillRunByID(ctx context.Context, id string) (*contracts.DrillRunRecord, error)

	SaveEvidenceBundle(ctx context.Context, bundle contracts.EvidenceBundleRecord) error
	GetEvidenceBundleByID(ctx context.Context, id string) (*contracts.EvidenceBundleRecord, error)

	SaveRestoreOp(ctx context.Context, op contracts.DRRestoreOperationRecord) error
	GetRestoreOpByID(ctx context.Context, tenantID string, id string) (*contracts.DRRestoreOperationRecord, error)

	SaveBreakglassGrant(ctx context.Context, grant contracts.BreakglassGrantRecord) error
	GetBreakglassGrantByID(ctx context.Context, tenantID string, id string) (*contracts.BreakglassGrantRecord, error)
}

type DatabaseFailoverPort interface {
	GetReplicationLag(ctx context.Context, storeType contracts.DRDataStoreType, primaryRegion, drRegion, dbInstance string) (int, error)
	InitiateSwitchover(ctx context.Context, storeType contracts.DRDataStoreType, drRegion, dbInstance string) error
	InitiateReplicaFailover(ctx context.Context, storeType contracts.DRDataStoreType, drRegion, dbInstance string) error
	FenceOldPrimary(ctx context.Context, storeType contracts.DRDataStoreType, primaryRegion, dbInstance string) error
	InitiateFailbackSwitchover(ctx context.Context, storeType contracts.DRDataStoreType, primaryRegion, dbInstance string) error
}

type TrafficRoutingPort interface {
	GetRoutingPolicy(ctx context.Context, planName string) (string, error)
	SwitchTrafficToDR(ctx context.Context, planName string, drRegion string) error
	LockRoutingPolicy(ctx context.Context, planName string) error
	FailbackTrafficToPrimary(ctx context.Context, planName string, primaryRegion string) error
}

type ClusterFailoverPort interface {
	VerifyClusterReadiness(ctx context.Context, clusterID string) (bool, error)
	ScaleWorkloads(ctx context.Context, clusterID string, topology contracts.DRTopology) error
	ExecuteRestorePlan(ctx context.Context, clusterID string, backupID string, namespace string) error
}

type SecretReplicationPort interface {
	VerifySecretParity(ctx context.Context, source contracts.DRSecretSyncSource, primaryRegion, drRegion string) (bool, error)
	SyncSecretsToDR(ctx context.Context, source contracts.DRSecretSyncSource, primaryRegion, drRegion string) error
}
