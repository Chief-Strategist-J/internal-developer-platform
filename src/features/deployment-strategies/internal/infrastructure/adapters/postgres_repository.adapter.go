/*
ALGORITHM & ARCHITECTURE BLUEPRINT: POSTGRES REPOSITORY ADAPTER (V.3 role 11, Invariant A, C)

1. Purpose:
   - Persists rollout plans, executions, and analysis run history in PostgreSQL.
   - Enforces Data Contract v1.2 tenant isolation and optimistic concurrency (version CAS).
2. Invariants:
   - Uses parameterized queries from queries package; emits OpenTelemetry client spans.
   - Zero inline comments below this header.
*/

package adapters

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/deployment-strategies/internal/contracts"
	"internal-developer-platform/src/features/deployment-strategies/internal/infrastructure/queries"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

func sanitizeUUID(val string) string {
	if _, err := uuid.Parse(val); err != nil {
		return uuid.New().String()
	}
	return val
}

type PostgresRepositoryAdapter struct {
	db *sql.DB
}

func NewPostgresRepositoryAdapter(db *sql.DB) *PostgresRepositoryAdapter {
	return &PostgresRepositoryAdapter{db: db}
}

func (a *PostgresRepositoryAdapter) CreatePlan(ctx context.Context, plan contracts.RolloutPlan) error {
	return observability.WithClientSpan(ctx, "deployment_strategies.postgres.create_plan", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("postgres database connection is nil")
		}
		stepsJSON, err := json.Marshal(plan.Steps)
		if err != nil {
			return fmt.Errorf("failed to marshal rollout plan steps: %w", err)
		}
		_, err = a.db.ExecContext(
			ctx,
			queries.QueryInsertPlan,
			plan.ID,
			plan.PublicID,
			plan.TenantID,
			plan.Name,
			string(plan.StrategyType),
			stepsJSON,
			plan.FailureThreshold,
			sanitizeUUID(plan.CorrelationID),
			plan.Version,
		)
		if err != nil {
			return fmt.Errorf("failed to insert rollout plan: %w", err)
		}
		return nil
	},
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "INSERT"),
		attribute.String("db.sql.table", "deployment_strategies.rollout_plans"),
		attribute.String("tenant.id", plan.TenantID),
	)
}

func (a *PostgresRepositoryAdapter) GetPlan(ctx context.Context, tenantID, planID string) (*contracts.RolloutPlan, error) {
	var plan contracts.RolloutPlan
	var strategyTypeStr string
	var stepsJSON []byte

	err := observability.WithClientSpan(ctx, "deployment_strategies.postgres.get_plan", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("postgres database connection is nil")
		}
		row := a.db.QueryRowContext(ctx, queries.QueryGetPlanByID, tenantID, planID)
		scanErr := row.Scan(
			&plan.ID,
			&plan.PublicID,
			&plan.TenantID,
			&plan.Name,
			&strategyTypeStr,
			&stepsJSON,
			&plan.FailureThreshold,
			&plan.CorrelationID,
			&plan.Version,
			&plan.CreatedAt,
			&plan.UpdatedAt,
		)
		if scanErr != nil {
			if scanErr == sql.ErrNoRows {
				return errors.NewNotFoundError(fmt.Sprintf("rollout plan %s not found", planID))
			}
			return fmt.Errorf("failed to query rollout plan: %w", scanErr)
		}
		return nil
	},
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "deployment_strategies.rollout_plans"),
	)

	if err != nil {
		return nil, err
	}
	plan.StrategyType = contracts.StrategyType(strategyTypeStr)
	if len(stepsJSON) > 0 {
		if unmarshalErr := json.Unmarshal(stepsJSON, &plan.Steps); unmarshalErr != nil {
			return nil, fmt.Errorf("failed to unmarshal rollout plan steps: %w", unmarshalErr)
		}
	}
	return &plan, nil
}

func (a *PostgresRepositoryAdapter) CreateExecution(ctx context.Context, execution contracts.RolloutExecution) error {
	return observability.WithClientSpan(ctx, "deployment_strategies.postgres.create_execution", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("postgres database connection is nil")
		}
		_, err := a.db.ExecContext(
			ctx,
			queries.QueryInsertExecution,
			execution.ID,
			execution.PublicID,
			execution.TenantID,
			execution.PlanID,
			execution.WorkloadID,
			execution.TargetImage,
			string(execution.Status),
			execution.CurrentStepIndex,
			execution.CurrentWeightPct,
			execution.ErrorMessage,
			sanitizeUUID(execution.CorrelationID),
			execution.Version,
		)
		if err != nil {
			return fmt.Errorf("failed to insert rollout execution: %w", err)
		}
		return nil
	},
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "INSERT"),
		attribute.String("db.sql.table", "deployment_strategies.rollout_executions"),
		attribute.String("tenant.id", execution.TenantID),
	)
}

func (a *PostgresRepositoryAdapter) GetExecution(ctx context.Context, tenantID, executionID string) (*contracts.RolloutExecution, error) {
	var exec contracts.RolloutExecution
	var statusStr string

	err := observability.WithClientSpan(ctx, "deployment_strategies.postgres.get_execution", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("postgres database connection is nil")
		}
		row := a.db.QueryRowContext(ctx, queries.QueryGetExecutionByID, tenantID, executionID)
		scanErr := row.Scan(
			&exec.ID,
			&exec.PublicID,
			&exec.TenantID,
			&exec.PlanID,
			&exec.WorkloadID,
			&exec.TargetImage,
			&statusStr,
			&exec.CurrentStepIndex,
			&exec.CurrentWeightPct,
			&exec.ErrorMessage,
			&exec.CorrelationID,
			&exec.Version,
			&exec.CreatedAt,
			&exec.UpdatedAt,
		)
		if scanErr != nil {
			if scanErr == sql.ErrNoRows {
				return errors.NewNotFoundError(fmt.Sprintf("rollout execution %s not found", executionID))
			}
			return fmt.Errorf("failed to query rollout execution: %w", scanErr)
		}
		return nil
	},
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "deployment_strategies.rollout_executions"),
	)

	if err != nil {
		return nil, err
	}
	exec.Status = contracts.RolloutStatus(statusStr)
	return &exec, nil
}

func (a *PostgresRepositoryAdapter) GetActiveExecutionForWorkload(ctx context.Context, tenantID, workloadID string) (*contracts.RolloutExecution, error) {
	var exec contracts.RolloutExecution
	var statusStr string

	err := observability.WithClientSpan(ctx, "deployment_strategies.postgres.get_active_execution", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("postgres database connection is nil")
		}
		row := a.db.QueryRowContext(ctx, queries.QueryGetActiveExecutionForWorkload, tenantID, workloadID)
		scanErr := row.Scan(
			&exec.ID,
			&exec.PublicID,
			&exec.TenantID,
			&exec.PlanID,
			&exec.WorkloadID,
			&exec.TargetImage,
			&statusStr,
			&exec.CurrentStepIndex,
			&exec.CurrentWeightPct,
			&exec.ErrorMessage,
			&exec.CorrelationID,
			&exec.Version,
			&exec.CreatedAt,
			&exec.UpdatedAt,
		)
		if scanErr != nil {
			if scanErr == sql.ErrNoRows {
				return nil
			}
			return fmt.Errorf("failed to query active rollout execution: %w", scanErr)
		}
		return nil
	},
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "deployment_strategies.rollout_executions"),
	)

	if err != nil {
		return nil, err
	}
	if exec.ID == "" {
		return nil, nil
	}
	exec.Status = contracts.RolloutStatus(statusStr)
	return &exec, nil
}

func (a *PostgresRepositoryAdapter) UpdateExecutionStatus(ctx context.Context, tenantID, executionID string, status contracts.RolloutStatus, stepIndex, weightPct int, errorMsg string, expectedVersion int64) error {
	return observability.WithClientSpan(ctx, "deployment_strategies.postgres.update_execution_status", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("postgres database connection is nil")
		}
		res, err := a.db.ExecContext(
			ctx,
			queries.QueryUpdateExecutionStatus,
			string(status),
			stepIndex,
			weightPct,
			errorMsg,
			tenantID,
			executionID,
			expectedVersion,
		)
		if err != nil {
			return fmt.Errorf("failed to update rollout execution status: %w", err)
		}
		rowsAffected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to verify rows affected: %w", err)
		}
		if rowsAffected == 0 {
			return errors.NewConflictError("optimistic concurrency conflict: execution version mismatch")
		}
		return nil
	},
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "UPDATE"),
		attribute.String("db.sql.table", "deployment_strategies.rollout_executions"),
		attribute.String("execution.status", string(status)),
	)
}

func (a *PostgresRepositoryAdapter) RecordAnalysisRun(ctx context.Context, run contracts.RolloutAnalysisRun) error {
	return observability.WithClientSpan(ctx, "deployment_strategies.postgres.record_analysis_run", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("postgres database connection is nil")
		}
		_, err := a.db.ExecContext(
			ctx,
			queries.QueryInsertAnalysisRun,
			run.ID,
			run.PublicID,
			run.TenantID,
			run.ExecutionID,
			run.MetricName,
			run.Query,
			run.MeasuredValue,
			run.ThresholdValue,
			string(run.Verdict),
			sanitizeUUID(run.CorrelationID),
		)
		if err != nil {
			return fmt.Errorf("failed to record analysis run: %w", err)
		}
		return nil
	},
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "INSERT"),
		attribute.String("db.sql.table", "deployment_strategies.rollout_analysis_runs"),
		attribute.String("analysis.verdict", string(run.Verdict)),
	)
}
