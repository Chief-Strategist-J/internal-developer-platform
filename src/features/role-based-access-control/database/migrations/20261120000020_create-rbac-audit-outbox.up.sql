-- migration:      0020
-- description:    create rbac_audit_outbox table for durable RBAC decision audit emission
-- author:         platform-core
-- date:           2026-11-25T16:55:00Z
-- depends_on:     0002, 0019
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         persists transactional outbox events for RBAC PERMIT, DENY, and ELEVATED_PERMIT decisions
-- entity:         rbac_audit_outbox
-- contract_rev:   1.0
-- lock_timeout:   3s
-- recipe:         CHECK | NOT NULL

SET lock_timeout = '3s';
SET statement_timeout = '5min';

CREATE TABLE IF NOT EXISTS rbac_audit_outbox (
    id VARCHAR(128) PRIMARY KEY CHECK (id <> ''),
    tenant_id VARCHAR(128) NOT NULL DEFAULT '',
    subject_id VARCHAR(255) NOT NULL DEFAULT '',
    resource VARCHAR(128) NOT NULL CHECK (resource <> ''),
    action VARCHAR(64) NOT NULL CHECK (action <> ''),
    decision VARCHAR(64) NOT NULL CHECK (decision IN ('PERMIT', 'DENY', 'ELEVATED_PERMIT')),
    reason TEXT NOT NULL DEFAULT '',
    violation_code VARCHAR(128) NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_rbac_audit_outbox_unpublished ON rbac_audit_outbox (occurred_at) WHERE published_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_rbac_audit_outbox_tenant_subject ON rbac_audit_outbox (tenant_id, subject_id, occurred_at DESC);
