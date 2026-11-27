/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING PORTS (V.3 role 10)

 1. Role:
    - Defines outbound ports for cluster and cloud resource provisioning clients.
 2. Invariants:
    - Zero inline comments; context-first IO signatures.
*/
package ports

import (
	"context"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
)

type ClusterProvisionerPort interface {
	ProvisionCluster(ctx context.Context, spec contracts.ClusterSpec) (*contracts.ClusterRecord, error)
	GetCluster(ctx context.Context, clusterID string) (*contracts.ClusterRecord, error)
	ValidatePosture(ctx context.Context, clusterID string) (*contracts.ClusterPostureReport, error)
}

type CloudResourceProvisionerPort interface {
	ProvisionWorkloadIdentity(ctx context.Context, spec contracts.WorkloadIdentitySpec) (*contracts.ResourceManifestResult, error)
	ReconcileLedger(ctx context.Context, activeWorkloadIDs []string) (*contracts.LedgerReconciliationReport, error)
}

type ProvisioningLedgerPort interface {
	RecordIntent(ctx context.Context, intent contracts.ProvisioningIntent) error
	RecordOutcome(ctx context.Context, intentID string, attempt int, status string, outcomeID string, errMsg string) error
	ListIntents(ctx context.Context, tenantID string, limit int) ([]contracts.ProvisioningIntent, error)
}
