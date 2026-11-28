-- migration:      20261128130000
-- description:    create provisioning approvals table for INV-09 approval integrity and INV-10 plan pinning
-- author:         platform-core
-- date:           2026-11-28T13:00:00Z
-- depends_on:     20261128124000
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         enforce approver != requester, plan hash pinning, expiry, and aggregate period budget
-- entity:         self_service_provisioning.provisioning_approvals
-- contract_rev:   1.2
-- lock_timeout:   3s
-- recipe:         TABLE | CHECK | RLS

SET lock_timeout = '3s';
SET statement_timeout = '5min';

CREATE TABLE IF NOT EXISTS self_service_provisioning.provisioning_approvals (
    id TEXT PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    requester_id TEXT NOT NULL,
    approver_id TEXT NOT NULL,
    plan_hash TEXT NOT NULL,
    template_sha TEXT NOT NULL,
    policy_sha TEXT NOT NULL,
    budget_units INTEGER NOT NULL DEFAULT 1,
    status TEXT NOT NULL DEFAULT 'APPROVED',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ssp_app_public_id_fmt_ck CHECK (public_id ~ '^sspa_[0-9a-f]{26}$'),
    CONSTRAINT ssp_app_tenant_trimmed_ck CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
    CONSTRAINT ssp_app_no_self_approval_ck CHECK (requester_id <> '' AND approver_id <> '' AND approver_id <> requester_id),
    CONSTRAINT ssp_app_plan_hash_ck CHECK (plan_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ssp_app_budget_pos_ck CHECK (budget_units >= 1),
    CONSTRAINT ssp_app_status_ck CHECK (status IN ('APPROVED', 'CONSUMED', 'REVOKED', 'EXPIRED'))
);

CREATE INDEX IF NOT EXISTS ssp_app_tenant_period_idx
    ON self_service_provisioning.provisioning_approvals (tenant_id, status, created_at);

ALTER TABLE self_service_provisioning.provisioning_approvals ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS ssp_approvals_tenant_isolation ON self_service_provisioning.provisioning_approvals;
CREATE POLICY ssp_approvals_tenant_isolation ON self_service_provisioning.provisioning_approvals
    USING (
        current_setting('app.current_tenant_id', true) IS NULL
        OR current_setting('app.current_tenant_id', true) = ''
        OR tenant_id = current_setting('app.current_tenant_id', true)
    );
