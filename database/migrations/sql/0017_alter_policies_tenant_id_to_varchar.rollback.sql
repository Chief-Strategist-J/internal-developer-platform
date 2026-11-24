-- migration_rollback: 0017
-- description:        revert tenant_id from VARCHAR(128) to UUID on policies, policy_exceptions, and policy_violations
-- author:             platform-security
-- date:               2026-11-24T12:30:00Z
-- reversible:         YES
-- lock_risk:          LOW
-- rows_affected:      schema only

BEGIN;

-- 1. Drop foreign key constraints referencing policies(tenant_id, id)
ALTER TABLE policy_exceptions DROP CONSTRAINT IF EXISTS policy_exceptions_policy_fk;
ALTER TABLE policy_violations DROP CONSTRAINT IF EXISTS policy_violations_policy_fk;

-- 2. Drop unique and primary key constraints depending on tenant_id
ALTER TABLE policy_exceptions DROP CONSTRAINT IF EXISTS policy_exceptions_tenant_id_id_pk;
ALTER TABLE policy_violations DROP CONSTRAINT IF EXISTS policy_violations_tenant_id_id_pk;
ALTER TABLE policy_violations DROP CONSTRAINT IF EXISTS policy_violations_tenant_source_external_uk;
ALTER TABLE policies DROP CONSTRAINT IF EXISTS policies_tenant_id_id_pk;
ALTER TABLE policies DROP CONSTRAINT IF EXISTS policies_tenant_source_ext_uk;

-- 3. Alter tenant_id column types back to UUID
ALTER TABLE policies ALTER COLUMN tenant_id TYPE UUID USING tenant_id::UUID;
ALTER TABLE policy_exceptions ALTER COLUMN tenant_id TYPE UUID USING tenant_id::UUID;
ALTER TABLE policy_violations ALTER COLUMN tenant_id TYPE UUID USING tenant_id::UUID;

-- 4. Re-add primary key and unique constraints
ALTER TABLE policies ADD CONSTRAINT policies_tenant_id_id_pk PRIMARY KEY (tenant_id, id);
ALTER TABLE policies ADD CONSTRAINT policies_tenant_source_ext_uk UNIQUE (tenant_id, source, external_id);
ALTER TABLE policy_exceptions ADD CONSTRAINT policy_exceptions_tenant_id_id_pk PRIMARY KEY (tenant_id, id);
ALTER TABLE policy_violations ADD CONSTRAINT policy_violations_tenant_id_id_pk PRIMARY KEY (tenant_id, id);
ALTER TABLE policy_violations ADD CONSTRAINT policy_violations_tenant_source_external_uk UNIQUE (tenant_id, source, external_id);

-- 5. Re-add composite foreign key constraints
ALTER TABLE policy_exceptions ADD CONSTRAINT policy_exceptions_policy_fk 
    FOREIGN KEY (tenant_id, policy_id) REFERENCES policies(tenant_id, id) ON DELETE RESTRICT;

ALTER TABLE policy_violations ADD CONSTRAINT policy_violations_policy_fk 
    FOREIGN KEY (tenant_id, policy_id) REFERENCES policies(tenant_id, id) ON DELETE RESTRICT;

-- 6. Ensure indexes exist
CREATE INDEX IF NOT EXISTS idx_policies_lifecycle ON policies (tenant_id, lifecycle_state);
CREATE INDEX IF NOT EXISTS idx_policies_enforcement ON policies (tenant_id, enforcement_point);
CREATE INDEX IF NOT EXISTS idx_policy_exceptions_policy ON policy_exceptions (tenant_id, policy_id);
CREATE INDEX IF NOT EXISTS idx_policy_exceptions_status ON policy_exceptions (tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_policy_exceptions_workload ON policy_exceptions (tenant_id, namespace, workload_id);
CREATE INDEX IF NOT EXISTS idx_policy_violations_lookup ON policy_violations (tenant_id, policy_id, status);
CREATE INDEX IF NOT EXISTS idx_policy_violations_workload ON policy_violations (tenant_id, namespace, workload_id, status);

COMMIT;
