package queries

const (
	FLOW_INSERT_RECONCILIATION = `
INSERT INTO gitops_reconciliations (
    id, tenant_id, service_id, app_name, target_revision, current_revision,
    sync_phase, health_status, idempotency_key, version, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
`

	FLOW_SELECT_RECONCILIATION_BY_ID = `
SELECT id, tenant_id, service_id, app_name, target_revision, current_revision,
       sync_phase, health_status, idempotency_key, version, created_at, updated_at
FROM gitops_reconciliations
WHERE tenant_id = $1 AND id = $2
`

	FLOW_SELECT_RECONCILIATION_BY_IDEMPOTENCY = `
SELECT id, tenant_id, service_id, app_name, target_revision, current_revision,
       sync_phase, health_status, idempotency_key, version, created_at, updated_at
FROM gitops_reconciliations
WHERE tenant_id = $1 AND idempotency_key = $2
`

	FLOW_UPDATE_RECONCILIATION = `
UPDATE gitops_reconciliations
SET current_revision = $1, sync_phase = $2, health_status = $3,
    version = version + 1, updated_at = $4
WHERE tenant_id = $5 AND id = $6 AND version = $7
`

	FLOW_LIST_RECONCILIATIONS = `
SELECT id, tenant_id, service_id, app_name, target_revision, current_revision,
       sync_phase, health_status, idempotency_key, version, created_at, updated_at
FROM gitops_reconciliations
WHERE tenant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3
`
)

