-- rollback for 0023_rbac_five_role_identifiers.sql
SET lock_timeout = '3s';
SET statement_timeout = '5min';

DROP TRIGGER IF EXISTS rbac_audit_outbox_immutable_trg ON rbac_audit_outbox;
DROP INDEX IF EXISTS rbac_audit_outbox_public_id_uq;
ALTER TABLE rbac_audit_outbox
    DROP COLUMN IF EXISTS correlation_id,
    DROP COLUMN IF EXISTS public_id;

DROP TRIGGER IF EXISTS rbac_user_bindings_immutable_trg ON rbac_user_bindings;
DROP INDEX IF EXISTS rbac_user_bindings_external_uq;
DROP INDEX IF EXISTS rbac_user_bindings_public_id_uq;
ALTER TABLE rbac_user_bindings
    DROP COLUMN IF EXISTS correlation_id,
    DROP COLUMN IF EXISTS external_id,
    DROP COLUMN IF EXISTS source,
    DROP COLUMN IF EXISTS public_id;

DROP FUNCTION IF EXISTS rbac_forbid_immutable_column_change();
