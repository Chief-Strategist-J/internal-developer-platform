package ports

import (
	"context"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
)

type WorkloadStorePort interface {
	Save(ctx context.Context, record contracts.WorkloadProvisionRecord) error
	GetByID(ctx context.Context, tenantID string, id string) (*contracts.WorkloadProvisionRecord, error)
	GetByIdempotencyKey(ctx context.Context, tenantID string, key string) (*contracts.WorkloadProvisionRecord, error)
	List(ctx context.Context, filter contracts.FilterCriteria) ([]contracts.WorkloadProvisionRecord, error)
	Update(ctx context.Context, record contracts.WorkloadProvisionRecord) error
}

type K8sClientPort interface {
	ApplyDeployment(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) error
	ApplyPDB(ctx context.Context, namespace string, workloadName string, minAvailable int) error
	GetWorkloadHealth(ctx context.Context, namespace string, workloadName string) (contracts.WorkloadStatus, error)
	GenerateManifestBundle(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) (*contracts.ManifestBundle, error)
	GetAppliedManifests(ctx context.Context, namespace string, workloadName string) (*contracts.ManifestBundle, error)
	GetNamespaceQuota(ctx context.Context, namespace string) (*contracts.ResourceQuotaSpec, error)
	ObserveWorkloadReadiness(ctx context.Context, namespace string, workloadName string, desiredReplicas int) (bool, error)
}

type GitOpsDeliveryPort interface {
	CommitManifestBundle(ctx context.Context, tenantID, workloadName string, bundle contracts.ManifestBundle) error
	RevertManifestBundle(ctx context.Context, tenantID, workloadName, reason string) error
}
