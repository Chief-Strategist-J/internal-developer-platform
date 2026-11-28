-- migration:      20261128133000
-- description:    add governance tables for INV-02/06/07 and EC-01/07/13/14
-- author:         platform-core
-- date:           2026-11-28T13:30:00Z
-- depends_on:     20261128130000
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         persist operation_id before polling, retention decisions, DNS cooldowns, and saga serialization locks
-- entity:         self_service_provisioning
-- contract_rev:   1.2
-- lock_timeout:   3s
-- recipe:         ALTER | TABLE | CHECK | RLS

SET lock_timeout = '3s';
SET statement_timeout = '5min';

ALTER TABLE self_service_provisioning.provisioning_intents
    ADD COLUMN IF NOT EXISTS operation_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS request_labels JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE TABLE IF NOT EXISTS self_service_provisioning.retention_decisions (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    resource_class TEXT NOT NULL,
    decision TEXT NOT NULL,
    decided_by TEXT NOT NULL,
    decided_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ssp_ret_decision_ck CHECK (decision IN ('RETAIN', 'ABANDON', 'DELETE_CONFIRMED')),
    CONSTRAINT ssp_ret_tenant_trimmed_ck CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id))
);

CREATE TABLE IF NOT EXISTS self_service_provisioning.dns_cooldowns (
    dns_name TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    released_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cooldown_until TIMESTAMPTZ NOT NULL,
    CONSTRAINT ssp_dns_name_norm_ck CHECK (dns_name <> '' AND dns_name = lower(btrim(dns_name)))
);

CREATE TABLE IF NOT EXISTS self_service_provisioning.saga_locks (
    service_key TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL,
    active_operation TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'RUNNING',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ssp_saga_op_ck CHECK (active_operation IN ('PROVISION', 'DECOMMISSION')),
    CONSTRAINT ssp_saga_state_ck CHECK (state IN ('RUNNING', 'ORPHANED_DIRTY', 'COMPLETED'))
);

ALTER TABLE self_service_provisioning.retention_decisions ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS ssp_retention_tenant_isolation ON self_service_provisioning.retention_decisions;
CREATE POLICY ssp_retention_tenant_isolation ON self_service_provisioning.retention_decisions
    USING (
        current_setting('app.current_tenant_id', true) IS NULL
        OR current_setting('app.current_tenant_id', true) = ''
        OR tenant_id = current_setting('app.current_tenant_id', true)
    );
