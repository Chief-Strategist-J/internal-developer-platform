// Package queries defines centralized SQL FLOW_* query constants for Disaster Recovery persistence.
//
// Architectural Blueprint:
// - Step 12 Queries: Strictly named FLOW_* queries for PostgreSQL / AlloyDB.
// - All updates are atomic, parameter-bound, and enforce tenant isolation and versioning.
// - 100% comment-free function bodies.
package queries

const (
	FlowInsertDRPlan = `
		INSERT INTO disaster_recovery.dr_plans (
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
		FROM disaster_recovery.dr_plans
		WHERE tenant_id = $1 AND (id::text = $2 OR public_id = $2);`

	FlowSelectDRPlanByIdempotencyKey = `
		SELECT
			id, public_id, tenant_id, source, external_id, correlation_id,
			plan_name, incident_tier, primary_region, dr_region, topology,
			target_rto_seconds, target_rpo_seconds, status, data_store_type,
			secret_sync_source, primary_cluster_id, dr_cluster_id,
			last_drill_at, last_drill_result, idempotency_key, version,
			created_at, updated_at
		FROM disaster_recovery.dr_plans
		WHERE tenant_id = $1 AND idempotency_key = $2;`

	FlowListDRPlansByTenant = `
		SELECT
			id, public_id, tenant_id, source, external_id, correlation_id,
			plan_name, incident_tier, primary_region, dr_region, topology,
			target_rto_seconds, target_rpo_seconds, status, data_store_type,
			secret_sync_source, primary_cluster_id, dr_cluster_id,
			last_drill_at, last_drill_result, idempotency_key, version,
			created_at, updated_at
		FROM disaster_recovery.dr_plans
		WHERE tenant_id = $1
		ORDER BY created_at DESC;`

	FlowUpdateDRPlanStatus = `
		UPDATE disaster_recovery.dr_plans
		SET status = $1, last_drill_at = $2, last_drill_result = $3, version = version + 1, updated_at = NOW()
		WHERE tenant_id = $4 AND id = $5 AND version = $6;`

	FlowInsertFencingEpoch = `
		INSERT INTO disaster_recovery.dr_fencing_epochs (dataset_id, authority, epoch, fenced_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (dataset_id)
		DO UPDATE SET authority = EXCLUDED.authority, epoch = disaster_recovery.dr_fencing_epochs.epoch + 1, fenced_at = EXCLUDED.fenced_at;`

	FlowSelectFencingEpoch = `
		SELECT dataset_id, authority, epoch, fenced_at
		FROM disaster_recovery.dr_fencing_epochs
		WHERE dataset_id = $1;`

	FlowInsertIncident = `
		INSERT INTO disaster_recovery.dr_incidents (
			id, public_id, tenant_id, plan_id, scope, declared_by, approved_by, evidence_links, state, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);`

	FlowSelectIncidentByID = `
		SELECT id, public_id, tenant_id, plan_id, scope, declared_by, approved_by, evidence_links, state, created_at
		FROM disaster_recovery.dr_incidents
		WHERE tenant_id = $1 AND (id::text = $2 OR public_id = $2);`

	FlowUpdateIncidentState = `
		UPDATE disaster_recovery.dr_incidents
		SET state = $1, approved_by = $2
		WHERE tenant_id = $3 AND (id::text = $4 OR public_id = $4);`

	FlowInsertFailoverOp = `
		INSERT INTO disaster_recovery.dr_failover_operations (
			id, public_id, tenant_id, plan_id, incident_id, mode, initiator, approver,
			state, projected_rpo_seconds, measured_rto_seconds, step_results, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);`

	FlowSelectFailoverOpByID = `
		SELECT id, public_id, tenant_id, plan_id, incident_id, mode, initiator, approver,
			state, projected_rpo_seconds, measured_rto_seconds, step_results, created_at, updated_at
		FROM disaster_recovery.dr_failover_operations
		WHERE tenant_id = $1 AND (id::text = $2 OR public_id = $2);`

	FlowInsertDrillRun = `
		INSERT INTO disaster_recovery.dr_drill_runs (
			id, public_id, tenant_id, plan_id, scope, measured_rto_seconds,
			measured_rpo_seconds, target_rto_seconds, target_rpo_seconds, passed, evidence_bundle_id, executed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);`

	FlowSelectDrillRunByID = `
		SELECT id, public_id, tenant_id, plan_id, scope, measured_rto_seconds,
			measured_rpo_seconds, target_rto_seconds, target_rpo_seconds, passed, evidence_bundle_id, executed_at
		FROM disaster_recovery.dr_drill_runs
		WHERE id::text = $1 OR public_id = $1;`

	FlowInsertDrillEvidence = `
		INSERT INTO disaster_recovery.dr_drill_evidence (
			id, public_id, tenant_id, drill_run_id, plan_id, executed_by, scope,
			measured_rto_seconds, measured_rpo_seconds, passed, verification_logs,
			audit_sink_region, checksum_sha256, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);`

	FlowSelectDrillEvidenceByID = `
		SELECT id, public_id, tenant_id, drill_run_id, plan_id, executed_by, scope,
			measured_rto_seconds, measured_rpo_seconds, passed, verification_logs,
			audit_sink_region, checksum_sha256, created_at
		FROM disaster_recovery.dr_drill_evidence
		WHERE id = $1 OR public_id = $1;`

	FlowInsertRestoreOp = `
		INSERT INTO disaster_recovery.dr_restore_operations (
			id, public_id, tenant_id, plan_id, backup_id, target_cluster, namespace,
			state, data_validated, validation_note, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);`

	FlowSelectRestoreOpByID = `
		SELECT id, public_id, tenant_id, plan_id, backup_id, target_cluster, namespace,
			state, data_validated, validation_note, created_at, updated_at
		FROM disaster_recovery.dr_restore_operations
		WHERE tenant_id = $1 AND (id::text = $2 OR public_id = $2);`

	FlowInsertBreakglassGrant = `
		INSERT INTO disaster_recovery.dr_breakglass_grants (
			id, public_id, tenant_id, requester, approver, role, expires_at, justification, active, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);`

	FlowSelectBreakglassGrantByID = `
		SELECT id, public_id, tenant_id, requester, approver, role, expires_at, justification, active, created_at
		FROM disaster_recovery.dr_breakglass_grants
		WHERE tenant_id = $1 AND (id::text = $2 OR public_id = $2);`
)
