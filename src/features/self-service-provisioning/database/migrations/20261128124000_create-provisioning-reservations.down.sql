SET lock_timeout = '3s';
SET statement_timeout = '5min';

DROP POLICY IF EXISTS ssp_reservations_tenant_isolation ON self_service_provisioning.provisioning_reservations;
DROP INDEX IF EXISTS self_service_provisioning.ssp_res_tenant_status_idx;
DROP INDEX IF EXISTS self_service_provisioning.ssp_res_active_name_uq;
DROP TABLE IF EXISTS self_service_provisioning.provisioning_reservations;
DROP TABLE IF EXISTS self_service_provisioning.tenant_capacity_ceilings;
