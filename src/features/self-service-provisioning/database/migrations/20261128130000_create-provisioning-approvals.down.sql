SET lock_timeout = '3s';
SET statement_timeout = '5min';

DROP POLICY IF EXISTS ssp_approvals_tenant_isolation ON self_service_provisioning.provisioning_approvals;
DROP INDEX IF EXISTS self_service_provisioning.ssp_app_tenant_period_idx;
DROP TABLE IF EXISTS self_service_provisioning.provisioning_approvals;
