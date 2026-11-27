DROP POLICY IF EXISTS ssp_intents_tenant_isolation ON self_service_provisioning.provisioning_intents;
ALTER TABLE self_service_provisioning.provisioning_intents DISABLE ROW LEVEL SECURITY;
DROP TRIGGER IF EXISTS ssp_intents_immutable_trg ON self_service_provisioning.provisioning_intents;
DROP FUNCTION IF EXISTS self_service_provisioning.forbid_immutable_intent_columns();
ALTER TABLE self_service_provisioning.provisioning_intents
    DROP CONSTRAINT IF EXISTS ssp_intents_version_pos_ck,
    DROP CONSTRAINT IF EXISTS ssp_intents_target_trimmed_ck,
    DROP CONSTRAINT IF EXISTS ssp_intents_tenant_trimmed_ck,
    DROP CONSTRAINT IF EXISTS ssp_intents_public_id_fmt_ck,
    DROP COLUMN IF EXISTS version,
    DROP COLUMN IF EXISTS correlation_id;
