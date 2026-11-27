-- Phase 5 Expand-Safe Table Ownership Relocation into disaster_recovery (D-01, D-02)
SET lock_timeout = '3s';
SET statement_timeout = '5min';

DO $$
BEGIN
    IF to_regclass('public.dr_plan_status') IS NOT NULL AND to_regclass('disaster_recovery.dr_plan_status') IS NULL THEN
        EXECUTE 'ALTER TABLE public.dr_plan_status SET SCHEMA disaster_recovery';
    END IF;
    IF to_regclass('public.dr_topologies') IS NOT NULL AND to_regclass('disaster_recovery.dr_topologies') IS NULL THEN
        EXECUTE 'ALTER TABLE public.dr_topologies SET SCHEMA disaster_recovery';
    END IF;
    IF to_regclass('public.dr_plans') IS NOT NULL AND to_regclass('disaster_recovery.dr_plans') IS NULL THEN
        EXECUTE 'ALTER TABLE public.dr_plans SET SCHEMA disaster_recovery';
    END IF;
    IF to_regclass('public.dr_fencing_epochs') IS NOT NULL AND to_regclass('disaster_recovery.dr_fencing_epochs') IS NULL THEN
        EXECUTE 'ALTER TABLE public.dr_fencing_epochs SET SCHEMA disaster_recovery';
    END IF;
    IF to_regclass('public.dr_incidents') IS NOT NULL AND to_regclass('disaster_recovery.dr_incidents') IS NULL THEN
        EXECUTE 'ALTER TABLE public.dr_incidents SET SCHEMA disaster_recovery';
    END IF;
    IF to_regclass('public.dr_failover_operations') IS NOT NULL AND to_regclass('disaster_recovery.dr_failover_operations') IS NULL THEN
        EXECUTE 'ALTER TABLE public.dr_failover_operations SET SCHEMA disaster_recovery';
    END IF;
    IF to_regclass('public.dr_drill_runs') IS NOT NULL AND to_regclass('disaster_recovery.dr_drill_runs') IS NULL THEN
        EXECUTE 'ALTER TABLE public.dr_drill_runs SET SCHEMA disaster_recovery';
    END IF;
    IF to_regclass('public.dr_drill_evidence') IS NOT NULL AND to_regclass('disaster_recovery.dr_drill_evidence') IS NULL THEN
        EXECUTE 'ALTER TABLE public.dr_drill_evidence SET SCHEMA disaster_recovery';
    END IF;
    IF to_regclass('public.dr_restore_operations') IS NOT NULL AND to_regclass('disaster_recovery.dr_restore_operations') IS NULL THEN
        EXECUTE 'ALTER TABLE public.dr_restore_operations SET SCHEMA disaster_recovery';
    END IF;
    IF to_regclass('public.dr_breakglass_grants') IS NOT NULL AND to_regclass('disaster_recovery.dr_breakglass_grants') IS NULL THEN
        EXECUTE 'ALTER TABLE public.dr_breakglass_grants SET SCHEMA disaster_recovery';
    END IF;
END $$;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA disaster_recovery TO svc_disaster_recovery;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA disaster_recovery TO svc_disaster_recovery;
