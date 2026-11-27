-- migration:      0023
-- description:    add 5-role identifier columns and immutability triggers to rbac tables
-- author:         platform-core
-- date:           2026-11-27T11:08:00Z
-- depends_on:     0022
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         align rbac_user_bindings and rbac_audit_outbox with 5-role identifier and immutability contract
-- entity:         rbac_user_bindings, rbac_audit_outbox
-- contract_rev:   1.0
-- lock_timeout:   3s
-- recipe:         CHECK | TRIGGER | UNIQUE

SET lock_timeout = '3s';
SET statement_timeout = '5min';

CREATE OR REPLACE FUNCTION rbac_forbid_immutable_column_change()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.public_id IS DISTINCT FROM OLD.public_id THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'public_id is immutable';
    END IF;
    IF NEW.correlation_id IS DISTINCT FROM OLD.correlation_id THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'correlation_id is immutable';
    END IF;
    IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'tenant_id is immutable';
    END IF;
    RETURN NEW;
END $$;

ALTER TABLE rbac_user_bindings
    ADD COLUMN IF NOT EXISTS public_id TEXT,
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'platform-api',
    ADD COLUMN IF NOT EXISTS external_id TEXT,
    ADD COLUMN IF NOT EXISTS correlation_id UUID NOT NULL DEFAULT gen_random_uuid();

UPDATE rbac_user_bindings
SET public_id = 'rbnd_' || md5(tenant_id || ':' || subject_id || ':' || role_name)
WHERE public_id IS NULL;

ALTER TABLE rbac_user_bindings
    ALTER COLUMN public_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS rbac_user_bindings_public_id_uq
    ON rbac_user_bindings (public_id);

CREATE UNIQUE INDEX IF NOT EXISTS rbac_user_bindings_external_uq
    ON rbac_user_bindings (tenant_id, source, external_id)
    WHERE external_id IS NOT NULL;

DROP TRIGGER IF EXISTS rbac_user_bindings_immutable_trg ON rbac_user_bindings;
CREATE TRIGGER rbac_user_bindings_immutable_trg
    BEFORE UPDATE ON rbac_user_bindings
    FOR EACH ROW EXECUTE FUNCTION rbac_forbid_immutable_column_change();

ALTER TABLE rbac_audit_outbox
    ADD COLUMN IF NOT EXISTS public_id TEXT,
    ADD COLUMN IF NOT EXISTS correlation_id UUID NOT NULL DEFAULT gen_random_uuid();

UPDATE rbac_audit_outbox
SET public_id = 'raud_' || md5(id || ':' || tenant_id)
WHERE public_id IS NULL;

ALTER TABLE rbac_audit_outbox
    ALTER COLUMN public_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS rbac_audit_outbox_public_id_uq
    ON rbac_audit_outbox (public_id);

DROP TRIGGER IF EXISTS rbac_audit_outbox_immutable_trg ON rbac_audit_outbox;
CREATE TRIGGER rbac_audit_outbox_immutable_trg
    BEFORE UPDATE ON rbac_audit_outbox
    FOR EACH ROW EXECUTE FUNCTION rbac_forbid_immutable_column_change();
