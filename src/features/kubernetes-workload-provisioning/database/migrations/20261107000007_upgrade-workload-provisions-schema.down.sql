SET lock_timeout = '3s';
SET statement_timeout = '5min';

-- migration rollback: 0007
-- description:        rollback workload provisions 05-db-writing-rules upgrades

DROP TRIGGER IF EXISTS trg_workload_provisions_immutability ON workload_provisions;
DROP FUNCTION IF EXISTS trg_assert_workload_identifiers_immutable();

ALTER TABLE workload_provisions
    DROP CONSTRAINT IF EXISTS fk_workload_provisions_status,
    DROP CONSTRAINT IF EXISTS fk_workload_provisions_strategy,
    DROP CONSTRAINT IF EXISTS uq_workload_provisions_public_id,
    DROP CONSTRAINT IF EXISTS uq_workload_provisions_natural_key,
    DROP CONSTRAINT IF EXISTS ck_workload_provisions_namespace_btrim,
    DROP CONSTRAINT IF EXISTS ck_workload_provisions_workload_name_btrim,
    DROP CONSTRAINT IF EXISTS ck_workload_provisions_source_btrim;

ALTER TABLE workload_provisions
    ADD CONSTRAINT chk_workload_strategy CHECK (strategy IN ('Canary', 'RollingUpdate', 'Recreate')),
    ADD CONSTRAINT chk_workload_status CHECK (status IN ('SUBMITTED', 'VALIDATED', 'ADMITTED', 'PROVISIONING', 'ACTIVE', 'OBSERVED_READY', 'DEGRADED', 'REJECTED'));

ALTER TABLE workload_provisions
    DROP COLUMN IF EXISTS public_id,
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS external_id,
    DROP COLUMN IF EXISTS correlation_id;

DROP TABLE IF EXISTS workload_lifecycle_states CASCADE;
DROP TABLE IF EXISTS workload_deployment_strategies CASCADE;

