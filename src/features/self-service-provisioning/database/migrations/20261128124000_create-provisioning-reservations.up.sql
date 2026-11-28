-- migration:      20261128124000
-- description:    create atomic reservation and tenant capacity ceiling tables (INV-04, INV-05, INV-15, EC-03)
-- author:         platform-core
-- date:           2026-11-28T12:40:00Z
-- depends_on:     20261127164000
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         enforce atomic name/quota/IP reservations and per-tenant capacity ceilings
-- entity:         self_service_provisioning.provisioning_reservations
-- contract_rev:   1.2
-- lock_timeout:   3s
-- recipe:         TABLE | UNIQUE_INDEX | CHECK | RLS

SET lock_timeout = '3s';
SET statement_timeout = '5min';

CREATE TABLE IF NOT EXISTS self_service_provisioning.tenant_capacity_ceilings (
    tenant_id TEXT PRIMARY KEY,
    max_quota_units INTEGER NOT NULL DEFAULT 100,
    max_ip_cidr_blocks INTEGER NOT NULL DEFAULT 64,
    max_cluster_ip_util_pct INTEGER NOT NULL DEFAULT 80,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ssp_ceilings_tenant_trimmed_ck CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
    CONSTRAINT ssp_ceilings_quota_pos_ck CHECK (max_quota_units >= 1),
    CONSTRAINT ssp_ceilings_ip_pos_ck CHECK (max_ip_cidr_blocks >= 1),
    CONSTRAINT ssp_ceilings_util_pct_ck CHECK (max_cluster_ip_util_pct BETWEEN 1 AND 100)
);

CREATE TABLE IF NOT EXISTS self_service_provisioning.provisioning_reservations (
    id TEXT PRIMARY KEY,
    public_id TEXT NOT NULL UNIQUE,
    tenant_id TEXT NOT NULL,
    resource_kind TEXT NOT NULL,
    normalized_name TEXT NOT NULL,
    quota_units INTEGER NOT NULL DEFAULT 1,
    ip_cidr_blocks INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'RESERVED',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ssp_res_public_id_fmt_ck CHECK (public_id ~ '^sspr_[0-9a-f]{26}$'),
    CONSTRAINT ssp_res_tenant_trimmed_ck CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
    CONSTRAINT ssp_res_name_norm_ck CHECK (normalized_name <> '' AND normalized_name = lower(btrim(normalized_name))),
    CONSTRAINT ssp_res_quota_pos_ck CHECK (quota_units >= 1),
    CONSTRAINT ssp_res_ip_nonneg_ck CHECK (ip_cidr_blocks >= 0),
    CONSTRAINT ssp_res_status_ck CHECK (status IN ('RESERVED', 'COMMITTED', 'RELEASED', 'EXPIRED'))
);

CREATE UNIQUE INDEX IF NOT EXISTS ssp_res_active_name_uq
    ON self_service_provisioning.provisioning_reservations (resource_kind, normalized_name)
    WHERE status IN ('RESERVED', 'COMMITTED');

CREATE INDEX IF NOT EXISTS ssp_res_tenant_status_idx
    ON self_service_provisioning.provisioning_reservations (tenant_id, status, expires_at);

ALTER TABLE self_service_provisioning.provisioning_reservations ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS ssp_reservations_tenant_isolation ON self_service_provisioning.provisioning_reservations;
CREATE POLICY ssp_reservations_tenant_isolation ON self_service_provisioning.provisioning_reservations
    USING (
        current_setting('app.current_tenant_id', true) IS NULL
        OR current_setting('app.current_tenant_id', true) = ''
        OR tenant_id = current_setting('app.current_tenant_id', true)
    );
