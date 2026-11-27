-- migration:      20261127164000
-- description:    harden self_service_provisioning.provisioning_intents per Data Contract v1.2
-- author:         platform-core
-- date:           2026-11-27T16:40:00Z
-- depends_on:     20261127163000
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         enforce 5-role identifiers, version CAS, trimmed text checks, immutability trigger, and RLS
-- entity:         self_service_provisioning.provisioning_intents
-- contract_rev:   1.2
-- lock_timeout:   3s
-- recipe:         CHECK | TRIGGER | RLS

SET lock_timeout = '3s';
SET statement_timeout = '5min';

ALTER TABLE self_service_provisioning.provisioning_intents
    ADD COLUMN IF NOT EXISTS correlation_id UUID NOT NULL DEFAULT gen_random_uuid(),
    ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1;

ALTER TABLE self_service_provisioning.provisioning_intents
    DROP CONSTRAINT IF EXISTS ssp_intents_public_id_fmt_ck,
    ADD CONSTRAINT ssp_intents_public_id_fmt_ck CHECK (public_id ~ '^sspi_[0-9a-f]{26}$') NOT VALID;
ALTER TABLE self_service_provisioning.provisioning_intents VALIDATE CONSTRAINT ssp_intents_public_id_fmt_ck;

ALTER TABLE self_service_provisioning.provisioning_intents
    DROP CONSTRAINT IF EXISTS ssp_intents_tenant_trimmed_ck,
    ADD CONSTRAINT ssp_intents_tenant_trimmed_ck CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)) NOT VALID;
ALTER TABLE self_service_provisioning.provisioning_intents VALIDATE CONSTRAINT ssp_intents_tenant_trimmed_ck;

ALTER TABLE self_service_provisioning.provisioning_intents
    DROP CONSTRAINT IF EXISTS ssp_intents_target_trimmed_ck,
    ADD CONSTRAINT ssp_intents_target_trimmed_ck CHECK (target_id <> '' AND target_id = btrim(target_id)) NOT VALID;
ALTER TABLE self_service_provisioning.provisioning_intents VALIDATE CONSTRAINT ssp_intents_target_trimmed_ck;

ALTER TABLE self_service_provisioning.provisioning_intents
    DROP CONSTRAINT IF EXISTS ssp_intents_version_pos_ck,
    ADD CONSTRAINT ssp_intents_version_pos_ck CHECK (version >= 1) NOT VALID;
ALTER TABLE self_service_provisioning.provisioning_intents VALIDATE CONSTRAINT ssp_intents_version_pos_ck;

CREATE OR REPLACE FUNCTION self_service_provisioning.forbid_immutable_intent_columns()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'id is immutable';
    END IF;
    IF NEW.public_id IS DISTINCT FROM OLD.public_id THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'public_id is immutable';
    END IF;
    IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'tenant_id is immutable';
    END IF;
    IF NEW.correlation_id IS DISTINCT FROM OLD.correlation_id THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'correlation_id is immutable';
    END IF;
    IF NEW.resource_kind IS DISTINCT FROM OLD.resource_kind THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'resource_kind is immutable';
    END IF;
    RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS ssp_intents_immutable_trg ON self_service_provisioning.provisioning_intents;
CREATE TRIGGER ssp_intents_immutable_trg
    BEFORE UPDATE ON self_service_provisioning.provisioning_intents
    FOR EACH ROW EXECUTE FUNCTION self_service_provisioning.forbid_immutable_intent_columns();

ALTER TABLE self_service_provisioning.provisioning_intents ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS ssp_intents_tenant_isolation ON self_service_provisioning.provisioning_intents;
CREATE POLICY ssp_intents_tenant_isolation ON self_service_provisioning.provisioning_intents
    USING (
        current_setting('app.current_tenant_id', true) IS NULL
        OR current_setting('app.current_tenant_id', true) = ''
        OR tenant_id = current_setting('app.current_tenant_id', true)
    );
