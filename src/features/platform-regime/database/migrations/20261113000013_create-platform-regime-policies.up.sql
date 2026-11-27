SET lock_timeout = '3s';
SET statement_timeout = '5min';

-- migration:      0013
-- description:    create platform regime policies table and lookup references
-- author:         chief-strategist
-- date:           2026-11-22T17:40:00Z
-- depends_on:     0012
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         implement dynamic platform regime governance (B-1, B-2, B-3, B-4) per data contract v1.2

CREATE TABLE IF NOT EXISTS regime_kms_custody_modes (
    code TEXT PRIMARY KEY,
    description TEXT NOT NULL
);

INSERT INTO regime_kms_custody_modes (code, description) VALUES
    ('CMEK', 'Customer-Managed Encryption Keys via Cloud KMS'),
    ('EKM', 'External Key Management via third-party HSM'),
    ('DOD_KMS', 'Dedicated Government Hardware Security Module')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS regime_cluster_modes (
    code TEXT PRIMARY KEY,
    description TEXT NOT NULL
);

INSERT INTO regime_cluster_modes (code, description) VALUES
    ('GKE_STANDARD', 'Google Kubernetes Engine Standard clusters with custom node pools'),
    ('GKE_AUTOPILOT', 'Google Kubernetes Engine Autopilot fully managed clusters')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS regime_tenancy_modes (
    code TEXT PRIMARY KEY,
    description TEXT NOT NULL
);

INSERT INTO regime_tenancy_modes (code, description) VALUES
    ('INTERNAL_ONLY', 'Internal government teams only with namespace-level isolation'),
    ('SHARED_EXTERNAL', 'Multi-tenant clusters shared with external vendors via strict network isolation')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS regime_policy_status (
    code TEXT PRIMARY KEY,
    description TEXT NOT NULL
);

INSERT INTO regime_policy_status (code, description) VALUES
    ('DRAFT', 'Policy draft under review'),
    ('ACTIVE', 'Active platform regime policy enforcing runtime floors'),
    ('SUPERSEDED', 'Historical superseded policy version')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS platform_regime_policies (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    public_id TEXT NOT NULL,
    tenant_id TEXT NOT NULL,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    correlation_id UUID NOT NULL DEFAULT gen_random_uuid(),
    jurisdiction TEXT NOT NULL,
    audit_retention_days INT NOT NULL,
    data_residency_regions JSONB NOT NULL DEFAULT '[]',
    kms_key_custody_mode TEXT NOT NULL,
    rto_minutes_tier0 INT NOT NULL,
    rpo_minutes_tier0 INT NOT NULL,
    rto_minutes_tier1 INT NOT NULL,
    rpo_minutes_tier1 INT NOT NULL,
    cluster_mode TEXT NOT NULL,
    primary_region_count INT NOT NULL,
    standby_region_count INT NOT NULL,
    tenant_isolation_mode TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, id),
    CONSTRAINT uq_platform_regime_public_id UNIQUE (public_id),
    CONSTRAINT uq_platform_regime_natural_key UNIQUE (tenant_id, source, external_id),
    CONSTRAINT fk_regime_kms_mode FOREIGN KEY (kms_key_custody_mode) REFERENCES regime_kms_custody_modes(code) ON DELETE RESTRICT,
    CONSTRAINT fk_regime_cluster_mode FOREIGN KEY (cluster_mode) REFERENCES regime_cluster_modes(code) ON DELETE RESTRICT,
    CONSTRAINT fk_regime_tenancy_mode FOREIGN KEY (tenant_isolation_mode) REFERENCES regime_tenancy_modes(code) ON DELETE RESTRICT,
    CONSTRAINT fk_regime_status FOREIGN KEY (status) REFERENCES regime_policy_status(code) ON DELETE RESTRICT,
    CONSTRAINT ck_regime_audit_retention_floor CHECK (audit_retention_days >= 30),
    CONSTRAINT ck_regime_rto_tier0_positive CHECK (rto_minutes_tier0 > 0),
    CONSTRAINT ck_regime_rpo_tier0_nonneg CHECK (rpo_minutes_tier0 >= 0),
    CONSTRAINT ck_regime_rto_tier1_positive CHECK (rto_minutes_tier1 > 0),
    CONSTRAINT ck_regime_rpo_tier1_nonneg CHECK (rpo_minutes_tier1 >= 0),
    CONSTRAINT ck_regime_primary_regions CHECK (primary_region_count >= 1),
    CONSTRAINT ck_regime_standby_regions CHECK (standby_region_count >= 1),
    CONSTRAINT ck_regime_regions_json_array CHECK (jsonb_typeof(data_residency_regions) = 'array'),
    CONSTRAINT ck_regime_jurisdiction_btrim CHECK (jurisdiction = btrim(jurisdiction) AND jurisdiction <> ''),
    CONSTRAINT ck_regime_source_btrim CHECK (source = btrim(source) AND source <> '')
);

CREATE INDEX IF NOT EXISTS idx_platform_regime_status ON platform_regime_policies (tenant_id, status);

CREATE OR REPLACE FUNCTION trg_assert_platform_regime_immutable()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.id IS DISTINCT FROM NEW.id THEN
        RAISE EXCEPTION 'platform_regime_policies.id is immutable';
    END IF;
    IF OLD.public_id IS DISTINCT FROM NEW.public_id THEN
        RAISE EXCEPTION 'platform_regime_policies.public_id is immutable';
    END IF;
    IF OLD.tenant_id IS DISTINCT FROM NEW.tenant_id THEN
        RAISE EXCEPTION 'platform_regime_policies.tenant_id is immutable';
    END IF;
    IF OLD.correlation_id IS DISTINCT FROM NEW.correlation_id THEN
        RAISE EXCEPTION 'platform_regime_policies.correlation_id is immutable';
    END IF;
    NEW.updated_at = NOW();
    NEW.version = OLD.version + 1;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_platform_regime_immutability ON platform_regime_policies;
CREATE TRIGGER trg_platform_regime_immutability
    BEFORE UPDATE ON platform_regime_policies
    FOR EACH ROW
    EXECUTE FUNCTION trg_assert_platform_regime_immutable();
