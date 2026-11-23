package ports

import (
	"context"
	"time"

	"internal-developer-platform/src/features/policy-as-code/contracts"
)

type PolicyRepositoryPort interface {
	SavePolicy(ctx context.Context, policy contracts.PolicyRecord) error
	GetPolicyByID(ctx context.Context, tenantID string, id string) (*contracts.PolicyRecord, error)
	GetPolicyByPublicID(ctx context.Context, tenantID string, publicID string) (*contracts.PolicyRecord, error)
	ListPoliciesKeyset(ctx context.Context, filter contracts.KeysetFilterCriteria) ([]contracts.PolicyRecord, error)
	ListActiveCELPolicies(ctx context.Context, tenantID string) ([]contracts.PolicyRecord, error)
	UpdatePolicy(ctx context.Context, policy contracts.PolicyRecord, expectedVersion int64) error
	UpdatePolicyLifecycle(ctx context.Context, tenantID string, id string, newState contracts.PolicyLifecycleState, expectedVersion int64) error
	SoftDeletePolicy(ctx context.Context, tenantID string, id string, expectedVersion int64) error
	SaveException(ctx context.Context, exception contracts.PolicyExceptionRecord) error
	GetExceptionByID(ctx context.Context, tenantID string, id string) (*contracts.PolicyExceptionRecord, error)
	GetActiveExceptionsForWorkload(ctx context.Context, tenantID string, namespace string, workloadID string) ([]contracts.PolicyExceptionRecord, error)
	UpdateExceptionStatus(ctx context.Context, tenantID string, id string, newStatus contracts.ExceptionStatus, expectedVersion int64) error
	ReconcileExpiredExceptions(ctx context.Context, tenantID string, now time.Time) (int, error)
	SaveViolation(ctx context.Context, v contracts.PolicyViolationRecord) error
	ListActiveViolationsByPolicy(ctx context.Context, tenantID string, policyID string) ([]contracts.PolicyViolationRecord, error)
	CountActiveViolationsByPolicy(ctx context.Context, tenantID string, policyID string) (int, error)
	ResolveViolation(ctx context.Context, tenantID string, violationID string, expectedVersion int64) error
}
