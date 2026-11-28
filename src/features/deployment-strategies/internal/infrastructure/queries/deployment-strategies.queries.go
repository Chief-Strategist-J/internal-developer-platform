/*
ALGORITHM & ARCHITECTURE BLUEPRINT: CENTRALIZED SQL QUERIES (V.3 role 05, Invariant C)

1. Purpose:
   - Centralizes all SQL statements for deployment_strategies schema.
   - Fully qualified table references; parameterized queries to eliminate injection.
2. Invariants:
   - Zero inline comments below this header.
*/

package queries

const (
	QueryInsertPlan = `
INSERT INTO deployment_strategies.rollout_plans (
    id, public_id, tenant_id, name, strategy_type, steps, failure_threshold, correlation_id, version, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW());`

	QueryGetPlanByID = `
SELECT id, public_id, tenant_id, name, strategy_type, steps, failure_threshold, correlation_id, version, created_at, updated_at
FROM deployment_strategies.rollout_plans
WHERE tenant_id = $1 AND id = $2;`

	QueryInsertExecution = `
INSERT INTO deployment_strategies.rollout_executions (
    id, public_id, tenant_id, plan_id, workload_id, target_image, status, current_step_index, current_weight_pct, error_message, correlation_id, version, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW());`

	QueryGetExecutionByID = `
SELECT id, public_id, tenant_id, plan_id, workload_id, target_image, status, current_step_index, current_weight_pct, error_message, correlation_id, version, created_at, updated_at
FROM deployment_strategies.rollout_executions
WHERE tenant_id = $1 AND id = $2;`

	QueryGetActiveExecutionForWorkload = `
SELECT id, public_id, tenant_id, plan_id, workload_id, target_image, status, current_step_index, current_weight_pct, error_message, correlation_id, version, created_at, updated_at
FROM deployment_strategies.rollout_executions
WHERE tenant_id = $1 AND workload_id = $2 AND status NOT IN ('COMPLETED', 'ABORTED')
LIMIT 1;`

	QueryUpdateExecutionStatus = `
UPDATE deployment_strategies.rollout_executions
SET status = $1,
    current_step_index = $2,
    current_weight_pct = $3,
    error_message = $4,
    version = version + 1,
    updated_at = NOW()
WHERE tenant_id = $5 AND id = $6 AND version = $7;`

	QueryInsertAnalysisRun = `
INSERT INTO deployment_strategies.rollout_analysis_runs (
    id, public_id, tenant_id, execution_id, metric_name, query, measured_value, threshold_value, verdict, correlation_id, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW());`
)
