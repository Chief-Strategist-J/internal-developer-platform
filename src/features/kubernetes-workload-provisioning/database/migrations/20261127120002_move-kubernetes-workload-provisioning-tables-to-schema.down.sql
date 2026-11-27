SET lock_timeout = '3s';
SET statement_timeout = '5min';

DO $$
BEGIN
    IF to_regclass('kubernetes_workload_provisioning.workload_lifecycle_states') IS NOT NULL AND to_regclass('public.workload_lifecycle_states') IS NULL THEN
        EXECUTE 'ALTER TABLE kubernetes_workload_provisioning.workload_lifecycle_states SET SCHEMA public';
    END IF;
    IF to_regclass('kubernetes_workload_provisioning.workload_deployment_strategies') IS NOT NULL AND to_regclass('public.workload_deployment_strategies') IS NULL THEN
        EXECUTE 'ALTER TABLE kubernetes_workload_provisioning.workload_deployment_strategies SET SCHEMA public';
    END IF;
    IF to_regclass('kubernetes_workload_provisioning.workload_provisions') IS NOT NULL AND to_regclass('public.workload_provisions') IS NULL THEN
        EXECUTE 'ALTER TABLE kubernetes_workload_provisioning.workload_provisions SET SCHEMA public';
    END IF;
    IF to_regclass('kubernetes_workload_provisioning.incident_timeline_events') IS NOT NULL AND to_regclass('public.incident_timeline_events') IS NULL THEN
        EXECUTE 'ALTER TABLE kubernetes_workload_provisioning.incident_timeline_events SET SCHEMA public';
    END IF;
END $$;
