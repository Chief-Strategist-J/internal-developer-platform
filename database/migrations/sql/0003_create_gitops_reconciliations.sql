-- migration:      0003
-- description:    create gitops reconciliations table
-- author:         chief-strategist
-- date:           2026-11-12T18:25:00Z
-- depends_on:     0002
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         initial schema creation for gitops-reconciliation feature

CREATE TABLE IF NOT EXISTS gitops_reconciliations (
    id VARCHAR(128) NOT NULL,
    tenant_id VARCHAR(128) NOT NULL,
    service_id VARCHAR(128) NOT NULL,
    app_name VARCHAR(256) NOT NULL,
    target_revision VARCHAR(128) NOT NULL,
    current_revision VARCHAR(128) NOT NULL DEFAULT '',
    sync_phase VARCHAR(64) NOT NULL,
    health_status VARCHAR(64) NOT NULL,
    idempotency_key VARCHAR(256) NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT uq_gitops_reconciliations_idempotency UNIQUE (tenant_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_gitops_reconciliations_service ON gitops_reconciliations(tenant_id, service_id);
CREATE INDEX IF NOT EXISTS idx_gitops_reconciliations_phase ON gitops_reconciliations(sync_phase);
