-- rollback for 0022_validate_rbac_constraints.sql
SET lock_timeout = '3s';
SET statement_timeout = '5min';

ALTER TABLE rbac_user_bindings DROP CONSTRAINT IF EXISTS rbac_user_bindings_subject_trimmed_ck;
ALTER TABLE rbac_user_bindings DROP CONSTRAINT IF EXISTS rbac_user_bindings_tenant_trimmed_ck;
