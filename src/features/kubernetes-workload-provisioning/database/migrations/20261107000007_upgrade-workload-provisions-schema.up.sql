SET lock_timeout = '3s';
SET statement_timeout = '5min';

-- migration:      0007
-- description:    upgrade workload provisions table with 05-db-writing-rules compliance
-- author:         chief-strategist
-- date:           2026-11-20T10:50:00Z
-- depends_on:     0006
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema and lookup tables
-- reason:         adhere strictly to 05-db-writing-rules mandatory 5-identifier roles, lookup references, and immutability

CREATE TABLE IF NOT EXISTS workload_lifecycle_states (
    code TEXT PRIMARY KEY,
    description TEXT NOT NULL
);

INSERT INTO workload_lifecycle_states (code, description) VALUES
    ('SUBMITTED', 'Workload provision request submitted'),
    ('VALIDATED', 'Resource limits and policy rules validated'),
    ('ADMITTED', 'In-process admission verified and accepted'),
    ('PROVISIONING', 'GitOps manifests committed and syncing'),
    ('ACTIVE', 'Workload active and observed running'),
    ('OBSERVED_READY', 'Replicas observed ready on cluster'),
    ('DEGRADED', 'Workload degraded or unhealthy'),
    ('REJECTED', 'Workload rejected by admission or policy')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS workload_deployment_strategies (
    code TEXT PRIMARY KEY,
    description TEXT NOT NULL
);

INSERT INTO workload_deployment_strategies (code, description) VALUES
    ('Canary', 'Progressive traffic shift canary rollout'),
    ('RollingUpdate', 'Kubernetes standard rolling update'),
    ('Recreate', 'All-at-once recreate deployment')
ON CONFLICT (code) DO NOTHING;

ALTER TABLE workload_provisions
    ADD COLUMN IF NOT EXISTS public_id TEXT,
    ADD COLUMN IF NOT EXISTS source TEXT DEFAULT 'api',
    ADD COLUMN IF NOT EXISTS external_id TEXT,
    ADD COLUMN IF NOT EXISTS correlation_id UUID DEFAULT gen_random_uuid();

UPDATE workload_provisions
SET
    public_id = COALESCE(public_id, 'wld_' || SUBSTRING(REPLACE(id::text, '-', ''), 1, 26)),
    external_id = COALESCE(external_id, id::text),
    source = COALESCE(source, 'api'),
    correlation_id = COALESCE(correlation_id, gen_random_uuid())
WHERE public_id IS NULL OR external_id IS NULL OR source IS NULL OR correlation_id IS NULL;

ALTER TABLE workload_provisions
    ALTER COLUMN public_id SET NOT NULL,
    ALTER COLUMN source SET NOT NULL,
    ALTER COLUMN external_id SET NOT NULL,
    ALTER COLUMN correlation_id SET NOT NULL;

ALTER TABLE workload_provisions
    DROP CONSTRAINT IF EXISTS uq_workload_provisions_public_id,
    ADD CONSTRAINT uq_workload_provisions_public_id UNIQUE (public_id);

ALTER TABLE workload_provisions
    DROP CONSTRAINT IF EXISTS uq_workload_provisions_natural_key,
    ADD CONSTRAINT uq_workload_provisions_natural_key UNIQUE (tenant_id, source, external_id);

ALTER TABLE workload_provisions
    DROP CONSTRAINT IF EXISTS chk_workload_status,
    DROP CONSTRAINT IF EXISTS fk_workload_provisions_status,
    ADD CONSTRAINT fk_workload_provisions_status FOREIGN KEY (status) REFERENCES workload_lifecycle_states(code) ON DELETE RESTRICT;

ALTER TABLE workload_provisions
    DROP CONSTRAINT IF EXISTS chk_workload_strategy,
    DROP CONSTRAINT IF EXISTS fk_workload_provisions_strategy,
    ADD CONSTRAINT fk_workload_provisions_strategy FOREIGN KEY (strategy) REFERENCES workload_deployment_strategies(code) ON DELETE RESTRICT;

ALTER TABLE workload_provisions
    DROP CONSTRAINT IF EXISTS ck_workload_provisions_namespace_btrim,
    ADD CONSTRAINT ck_workload_provisions_namespace_btrim CHECK (namespace = btrim(namespace) AND namespace <> '');

ALTER TABLE workload_provisions
    DROP CONSTRAINT IF EXISTS ck_workload_provisions_workload_name_btrim,
    ADD CONSTRAINT ck_workload_provisions_workload_name_btrim CHECK (workload_name = btrim(workload_name) AND workload_name <> '');

ALTER TABLE workload_provisions
    DROP CONSTRAINT IF EXISTS ck_workload_provisions_source_btrim,
    ADD CONSTRAINT ck_workload_provisions_source_btrim CHECK (source = btrim(source) AND source <> '');

CREATE OR REPLACE FUNCTION trg_assert_workload_identifiers_immutable()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.id IS DISTINCT FROM NEW.id THEN
        RAISE EXCEPTION 'workload_provisions.id is immutable';
    END IF;
    IF OLD.public_id IS DISTINCT FROM NEW.public_id THEN
        RAISE EXCEPTION 'workload_provisions.public_id is immutable';
    END IF;
    IF OLD.tenant_id IS DISTINCT FROM NEW.tenant_id THEN
        RAISE EXCEPTION 'workload_provisions.tenant_id is immutable';
    END IF;
    IF OLD.correlation_id IS DISTINCT FROM NEW.correlation_id THEN
        RAISE EXCEPTION 'workload_provisions.correlation_id is immutable';
    END IF;
    NEW.updated_at = NOW();
    NEW.version = OLD.version + 1;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_workload_provisions_immutability ON workload_provisions;
CREATE TRIGGER trg_workload_provisions_immutability
    BEFORE UPDATE ON workload_provisions
    FOR EACH ROW
    EXECUTE FUNCTION trg_assert_workload_identifiers_immutable();

