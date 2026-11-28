package queries

const (
	FLOW_INSERT_RECONCILIATION = `
INSERT INTO gitops_reconciliation.gitops_reconciliations (
    id, tenant_id, service_id, app_name, target_revision, current_revision,
    sync_phase, health_status, idempotency_key, version, created_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
`

	FLOW_SELECT_RECONCILIATION_BY_ID = `
SELECT id, tenant_id, service_id, app_name, target_revision, current_revision,
       sync_phase, health_status, idempotency_key, version, created_at, updated_at
FROM gitops_reconciliation.gitops_reconciliations
WHERE tenant_id = $1 AND id = $2
`

	FLOW_SELECT_RECONCILIATION_BY_IDEMPOTENCY = `
SELECT id, tenant_id, service_id, app_name, target_revision, current_revision,
       sync_phase, health_status, idempotency_key, version, created_at, updated_at
FROM gitops_reconciliation.gitops_reconciliations
WHERE tenant_id = $1 AND idempotency_key = $2
`

	FLOW_SELECT_LATEST_SYNCED = `
SELECT id, tenant_id, service_id, app_name, target_revision, current_revision,
       sync_phase, health_status, idempotency_key, version, created_at, updated_at
FROM gitops_reconciliation.gitops_reconciliations
WHERE tenant_id = $1 AND service_id = $2 AND sync_phase = 'SYNCED'
ORDER BY updated_at DESC
LIMIT 1
`

	FLOW_UPDATE_RECONCILIATION = `
UPDATE gitops_reconciliation.gitops_reconciliations
SET current_revision = $1, sync_phase = $2, health_status = $3,
    version = version + 1, updated_at = $4
WHERE tenant_id = $5 AND id = $6 AND version = $7
`

	FLOW_LIST_RECONCILIATIONS = `
SELECT id, tenant_id, service_id, app_name, target_revision, current_revision,
       sync_phase, health_status, idempotency_key, version, created_at, updated_at
FROM gitops_reconciliation.gitops_reconciliations
WHERE tenant_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3
`

	FLOW_INSERT_OUTBOX_EVENT = `
INSERT INTO gitops_reconciliation.gitops_outbox_events (id, tenant_id, aggregate_id, event_type, app_name, revision, occurred_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`

	FLOW_FETCH_UNPUBLISHED_OUTBOX_EVENTS = `
SELECT id, tenant_id, aggregate_id, event_type, app_name, revision, occurred_at
FROM gitops_reconciliation.gitops_outbox_events
WHERE published_at IS NULL
ORDER BY occurred_at ASC
LIMIT $1
`

	FLOW_MARK_OUTBOX_EVENT_PUBLISHED = `
UPDATE gitops_reconciliation.gitops_outbox_events
SET published_at = NOW()
WHERE id = $1 AND published_at IS NULL
`

	FLOW_SELECT_SECRET_MECHANISM_CONFIG = `
SELECT tenant_id, mechanism, COALESCE(vault_endpoint, ''), COALESCE(cluster_scope, ''),
       COALESCE(secret_store_ref, ''), COALESCE(csi_driver_name, ''), COALESCE(encryption_key_ref, ''), enabled
FROM gitops_reconciliation.gitops_secret_mechanisms
WHERE tenant_id = $1 AND deleted_at IS NULL
`

	FLOW_UPSERT_SECRET_MECHANISM_CONFIG = `
INSERT INTO gitops_reconciliation.gitops_secret_mechanisms (
	tenant_id, mechanism, vault_endpoint, cluster_scope,
	secret_store_ref, csi_driver_name, encryption_key_ref, enabled, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
ON CONFLICT (tenant_id) DO UPDATE SET
	mechanism = EXCLUDED.mechanism,
	vault_endpoint = EXCLUDED.vault_endpoint,
	cluster_scope = EXCLUDED.cluster_scope,
	secret_store_ref = EXCLUDED.secret_store_ref,
	csi_driver_name = EXCLUDED.csi_driver_name,
	encryption_key_ref = EXCLUDED.encryption_key_ref,
	enabled = EXCLUDED.enabled,
	version = gitops_reconciliation.gitops_secret_mechanisms.version + 1,
	updated_at = NOW()
`
)
