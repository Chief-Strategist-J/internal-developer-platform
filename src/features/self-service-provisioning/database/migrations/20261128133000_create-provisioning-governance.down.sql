SET lock_timeout = '3s';
SET statement_timeout = '5min';

DROP POLICY IF EXISTS ssp_retention_tenant_isolation ON self_service_provisioning.retention_decisions;
DROP TABLE IF EXISTS self_service_provisioning.saga_locks;
DROP TABLE IF EXISTS self_service_provisioning.dns_cooldowns;
DROP TABLE IF EXISTS self_service_provisioning.retention_decisions;

ALTER TABLE self_service_provisioning.provisioning_intents
    DROP COLUMN IF EXISTS request_labels,
    DROP COLUMN IF EXISTS operation_id;
