-- Phase 5 Expand-Safe Table Ownership Relocation into kubernetes_workload_provisioning (D-01, D-02)
SET lock_timeout = '3s';
SET statement_timeout = '5min';

DO $$
BEGIN
    IF to_regclass('public.workload_lifecycle_states') IS NOT NULL AND to_regclass('kubernetes_workload_provisioning.workload_lifecycle_states') IS NULL THEN
        EXECUTE 'ALTER TABLE public.workload_lifecycle_states SET SCHEMA kubernetes_workload_provisioning';
    END IF;
    IF to_regclass('public.workload_deployment_strategies') IS NOT NULL AND to_regclass('kubernetes_workload_provisioning.workload_deployment_strategies') IS NULL THEN
        EXECUTE 'ALTER TABLE public.workload_deployment_strategies SET SCHEMA kubernetes_workload_provisioning';
    END IF;
    IF to_regclass('public.workload_provisions') IS NOT NULL AND to_regclass('kubernetes_workload_provisioning.workload_provisions') IS NULL THEN
        EXECUTE 'ALTER TABLE public.workload_provisions SET SCHEMA kubernetes_workload_provisioning';
    END IF;
    IF to_regclass('public.incident_timeline_events') IS NOT NULL AND to_regclass('kubernetes_workload_provisioning.incident_timeline_events') IS NULL THEN
        EXECUTE 'ALTER TABLE public.incident_timeline_events SET SCHEMA kubernetes_workload_provisioning';
    END IF;
END $$;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA kubernetes_workload_provisioning TO svc_workload_provisioning;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA kubernetes_workload_provisioning TO svc_workload_provisioning;
