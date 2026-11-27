-- migration:      0014
-- description:    create policy violations table and lookup references with 05-db-writing-rules compliance
-- author:         platform-security
-- date:           2026-11-23T15:45:00Z
-- depends_on:     0013
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         persists violation records and audit scan findings for policy-as-code
-- entity:         policy_violations
-- contract_rev:   1.0
-- lock_timeout:   3s
-- recipe:         CHECK | FK | NOT NULL | UNIQUE | TRIGGER

SET lock_timeout = '3s';
SET statement_timeout = '5min';

CREATE TABLE IF NOT EXISTS policy_violation_status (
    code TEXT PRIMARY KEY,
    description TEXT NOT NULL
);

INSERT INTO policy_violation_status (code, description) VALUES
    ('ACTIVE', 'Active unresolved policy violation'),
    ('RESOLVED', 'Violation resolved through workload compliance'),
    ('EXEMPTED', 'Violation suppressed under active exception lease')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS policy_violations (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    public_id TEXT NOT NULL,
    tenant_id UUID NOT NULL,
    policy_id UUID NOT NULL,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    correlation_id UUID NOT NULL DEFAULT gen_random_uuid(),
    workload_id TEXT NOT NULL,
    namespace TEXT NOT NULL,
    severity TEXT NOT NULL,
    enforcement_point TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'ACTIVE',
    message TEXT NOT NULL,
    detection_pair TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT policy_violations_tenant_id_id_pk PRIMARY KEY (tenant_id, id),
    CONSTRAINT policy_violations_public_id_uk UNIQUE (public_id),
    CONSTRAINT policy_violations_tenant_source_external_uk UNIQUE (tenant_id, source, external_id),
    CONSTRAINT policy_violations_policy_fk FOREIGN KEY (tenant_id, policy_id) REFERENCES policies(tenant_id, id) ON DELETE RESTRICT,
    CONSTRAINT policy_violations_status_fk FOREIGN KEY (status) REFERENCES policy_violation_status(code) ON DELETE RESTRICT,
    CONSTRAINT policy_violations_source_ck CHECK (source IN ('ADMISSION', 'AUDIT_SCAN', 'CI')),
    CONSTRAINT policy_violations_severity_ck CHECK (severity IN ('CRITICAL', 'HIGH', 'MEDIUM', 'ADVISORY')),
    CONSTRAINT policy_violations_enforcement_point_ck CHECK (enforcement_point IN ('CEL', 'PSA', 'PC', 'BA', 'ORG', 'CI', 'ACD')),
    CONSTRAINT policy_violations_workload_ck CHECK (workload_id = btrim(workload_id) AND workload_id <> '' AND length(workload_id) <= 256),
    CONSTRAINT policy_violations_namespace_ck CHECK (namespace = btrim(namespace) AND namespace <> '' AND length(namespace) <= 256),
    CONSTRAINT policy_violations_message_ck CHECK (message = btrim(message) AND message <> '' AND length(message) <= 1024),
    CONSTRAINT policy_violations_external_id_ck CHECK (external_id = btrim(external_id) AND external_id <> '' AND length(external_id) <= 256)
);

CREATE INDEX IF NOT EXISTS idx_policy_violations_lookup ON policy_violations (tenant_id, policy_id, status);
CREATE INDEX IF NOT EXISTS idx_policy_violations_workload ON policy_violations (tenant_id, namespace, workload_id, status);

CREATE OR REPLACE FUNCTION trg_assert_policy_violations_immutable()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.id IS DISTINCT FROM NEW.id THEN
        RAISE EXCEPTION 'policy_violations.id is immutable';
    END IF;
    IF OLD.public_id IS DISTINCT FROM NEW.public_id THEN
        RAISE EXCEPTION 'policy_violations.public_id is immutable';
    END IF;
    IF OLD.tenant_id IS DISTINCT FROM NEW.tenant_id THEN
        RAISE EXCEPTION 'policy_violations.tenant_id is immutable';
    END IF;
    IF OLD.policy_id IS DISTINCT FROM NEW.policy_id THEN
        RAISE EXCEPTION 'policy_violations.policy_id is immutable';
    END IF;
    IF OLD.correlation_id IS DISTINCT FROM NEW.correlation_id THEN
        RAISE EXCEPTION 'policy_violations.correlation_id is immutable';
    END IF;
    NEW.updated_at = NOW();
    NEW.version = OLD.version + 1;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_policy_violations_immutability ON policy_violations;
CREATE TRIGGER trg_policy_violations_immutability
    BEFORE UPDATE ON policy_violations
    FOR EACH ROW
    EXECUTE FUNCTION trg_assert_policy_violations_immutable();
