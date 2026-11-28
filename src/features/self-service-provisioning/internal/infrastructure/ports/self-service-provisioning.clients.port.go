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
	"time"

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

type ProvisioningReservationPort interface {
	Reserve(ctx context.Context, req contracts.ReservationRequest) (*contracts.ReservationRecord, error)
	Commit(ctx context.Context, reservationID string) error
	Release(ctx context.Context, reservationID string) error
	SetTenantCeiling(ctx context.Context, ceiling contracts.TenantCapacityCeiling) error
}

type ProvisioningApprovalPort interface {
	RecordApproval(ctx context.Context, approval contracts.ApprovalRecord) error
	VerifyAndConsumeApproval(ctx context.Context, approvalID string, expectedPlan contracts.PinnedPlan, maxPeriodBudget int) error
}

type ProvisioningGovernancePort interface {
	RecordOperationID(ctx context.Context, intentID string, operationID string) error
	RecordRetentionDecision(ctx context.Context, decision contracts.RetentionDecision) error
	HasRetentionDecision(ctx context.Context, tenantID string, resourceID string) (bool, error)
	EnterDNSCooldown(ctx context.Context, tenantID string, dnsName string, cooldown time.Duration) error
	IsDNSInCooldown(ctx context.Context, dnsName string) (bool, error)
	AcquireSagaLock(ctx context.Context, serviceKey string, tenantID string, operation string) error
	ReleaseSagaLock(ctx context.Context, serviceKey string, finalState string) error
	RunSweeper(ctx context.Context) (*contracts.SweeperReport, error)
}
