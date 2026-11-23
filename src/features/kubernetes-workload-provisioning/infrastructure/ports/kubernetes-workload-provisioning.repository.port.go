// Package ports declares outbound interface contracts for the
// Kubernetes Workload Provisioning feature.
//
// Architectural Blueprint:
// - Step 10 Ports: Abstract boundaries decoupling domain services from concrete infrastructure.
// - Covers storage persistence, Kubernetes API interactions, GitOps repository delivery, and KMS encryption.
// - All adapters in infrastructure/adapters/ fulfill these port contracts.
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
	RecordTimelineEvent(ctx context.Context, event contracts.IncidentTimelineEvent) error
	ListTimelineEvents(ctx context.Context, tenantID string, workloadID string, limit int, offset int) ([]contracts.IncidentTimelineEvent, error)
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

type KmsEncryptionPort interface {
	EncryptSecret(ctx context.Context, keyURI string, plaintext []byte, additionalData string) (string, error)
}
