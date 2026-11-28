/*
ALGORITHM & ARCHITECTURE BLUEPRINT: REPOSITORY PORT INTERFACE (V.3 role 06, Invariant A, B, Data Contract v1.2)

1. Purpose:
   - Driven port for PostgreSQL rollout ledger persistence.
2. Invariants:
   - Idempotent and version-fenced mutations; zero inline comments below header.
*/

package ports

import (
	"context"
	"internal-developer-platform/src/features/deployment-strategies/internal/contracts"
)

type RepositoryPort interface {
	CreatePlan(ctx context.Context, plan contracts.RolloutPlan) error
	GetPlan(ctx context.Context, tenantID, planID string) (*contracts.RolloutPlan, error)
	CreateExecution(ctx context.Context, execution contracts.RolloutExecution) error
	GetExecution(ctx context.Context, tenantID, executionID string) (*contracts.RolloutExecution, error)
	GetActiveExecutionForWorkload(ctx context.Context, tenantID, workloadID string) (*contracts.RolloutExecution, error)
	UpdateExecutionStatus(ctx context.Context, tenantID, executionID string, status contracts.RolloutStatus, stepIndex, weightPct int, errorMsg string, expectedVersion int64) error
	RecordAnalysisRun(ctx context.Context, run contracts.RolloutAnalysisRun) error
}
