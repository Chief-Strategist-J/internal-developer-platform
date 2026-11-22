-- migration:      0011
-- description:    create access_grants table for jit access sessions and debug controls
-- author:         chief-strategist
-- date:           2026-11-20T17:45:00Z
-- depends_on:     0010
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         enforce access and debug compliance floor (V-24, V-25, V-26, V-28, V-32) per business-logic.md

CREATE TABLE IF NOT EXISTS access_grants (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    subject_id VARCHAR(128) NOT NULL,
    target_namespace VARCHAR(64) NOT NULL,
    role VARCHAR(64) NOT NULL,
    reason TEXT NOT NULL,
    duration_seconds INT NOT NULL,
    approved_by VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    expires_at TIMESTAMPTZ NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_grant_status CHECK (status IN ('ACTIVE', 'EXPIRED', 'REVOKED')),
    CONSTRAINT chk_grant_duration CHECK (duration_seconds > 0 AND duration_seconds <= 14400),
    CONSTRAINT chk_prohibit_nodes_proxy CHECK (role NOT LIKE '%nodes/proxy%' AND role NOT LIKE '%nodes/exec%')
);

CREATE INDEX IF NOT EXISTS idx_access_grants_tenant_subject ON access_grants (tenant_id, subject_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_access_grants_idempotency ON access_grants (tenant_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_access_grants_expires ON access_grants (expires_at);

