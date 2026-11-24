// Package queries defines centralized SQL FLOW_* query constants for Disaster Recovery persistence.
//
// Architectural Blueprint:
// - Step 12 Queries: Strictly named FLOW_* queries for PostgreSQL / AlloyDB.
// - All updates are atomic, parameter-bound, and enforce tenant isolation and versioning.
// - 100% comment-free function bodies.
package queries

const (
	FlowInsertDRPlan = `
		INSERT INTO dr_plans (
			id, public_id, tenant_id, source, external_id, correlation_id,
			plan_name, incident_tier, primary_region, dr_region, topology,
			target_rto_seconds, target_rpo_seconds, status, data_store_type,
			secret_sync_source, primary_cluster_id, dr_cluster_id,
			idempotency_key, version, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11,
			$12, $13, $14, $15,
			$16, $17, $18,
			$19, $20, $21, $22
		);`

	FlowSelectDRPlanByID = `
		SELECT
			id, public_id, tenant_id, source, external_id, correlation_id,
			plan_name, incident_tier, primary_region, dr_region, topology,
			target_rto_seconds, target_rpo_seconds, status, data_store_type,
			secret_sync_source, primary_cluster_id, dr_cluster_id,
			last_drill_at, last_drill_result, idempotency_key, version,
			created_at, updated_at
		FROM dr_plans
		WHERE tenant_id = $1 AND (id = $2 OR public_id = $2);`

	FlowSelectDRPlanByIdempotencyKey = `
		SELECT
			id, public_id, tenant_id, source, external_id, correlation_id,
			plan_name, incident_tier, primary_region, dr_region, topology,
			target_rto_seconds, target_rpo_seconds, status, data_store_type,
			secret_sync_source, primary_cluster_id, dr_cluster_id,
			last_drill_at, last_drill_result, idempotency_key, version,
			created_at, updated_at
		FROM dr_plans
		WHERE tenant_id = $1 AND idempotency_key = $2;`

	FlowListDRPlansByTenant = `
		SELECT
			id, public_id, tenant_id, source, external_id, correlation_id,
			plan_name, incident_tier, primary_region, dr_region, topology,
			target_rto_seconds, target_rpo_seconds, status, data_store_type,
			secret_sync_source, primary_cluster_id, dr_cluster_id,
			last_drill_at, last_drill_result, idempotency_key, version,
			created_at, updated_at
		FROM dr_plans
		WHERE tenant_id = $1
		ORDER BY created_at DESC;`

	FlowUpdateDRPlanStatus = `
		UPDATE dr_plans
		SET status = $1, last_drill_at = $2, last_drill_result = $3, version = version + 1, updated_at = NOW()
		WHERE tenant_id = $4 AND id = $5 AND version = $6;`

	FlowInsertFencingEpoch = `
		INSERT INTO dr_fencing_epochs (dataset_id, authority, epoch, fenced_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (dataset_id)
		DO UPDATE SET authority = EXCLUDED.authority, epoch = dr_fencing_epochs.epoch + 1, fenced_at = EXCLUDED.fenced_at;`

	FlowSelectFencingEpoch = `
		SELECT dataset_id, authority, epoch, fenced_at
		FROM dr_fencing_epochs
		WHERE dataset_id = $1;`
)

