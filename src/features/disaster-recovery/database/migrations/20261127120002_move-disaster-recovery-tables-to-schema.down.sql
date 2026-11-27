SET lock_timeout = '3s';
SET statement_timeout = '5min';

DO $$
BEGIN
    IF to_regclass('disaster_recovery.dr_plan_status') IS NOT NULL AND to_regclass('public.dr_plan_status') IS NULL THEN
        EXECUTE 'ALTER TABLE disaster_recovery.dr_plan_status SET SCHEMA public';
    END IF;
    IF to_regclass('disaster_recovery.dr_topologies') IS NOT NULL AND to_regclass('public.dr_topologies') IS NULL THEN
        EXECUTE 'ALTER TABLE disaster_recovery.dr_topologies SET SCHEMA public';
    END IF;
    IF to_regclass('disaster_recovery.dr_plans') IS NOT NULL AND to_regclass('public.dr_plans') IS NULL THEN
        EXECUTE 'ALTER TABLE disaster_recovery.dr_plans SET SCHEMA public';
    END IF;
    IF to_regclass('disaster_recovery.dr_fencing_epochs') IS NOT NULL AND to_regclass('public.dr_fencing_epochs') IS NULL THEN
        EXECUTE 'ALTER TABLE disaster_recovery.dr_fencing_epochs SET SCHEMA public';
    END IF;
    IF to_regclass('disaster_recovery.dr_incidents') IS NOT NULL AND to_regclass('public.dr_incidents') IS NULL THEN
        EXECUTE 'ALTER TABLE disaster_recovery.dr_incidents SET SCHEMA public';
    END IF;
    IF to_regclass('disaster_recovery.dr_failover_operations') IS NOT NULL AND to_regclass('public.dr_failover_operations') IS NULL THEN
        EXECUTE 'ALTER TABLE disaster_recovery.dr_failover_operations SET SCHEMA public';
    END IF;
    IF to_regclass('disaster_recovery.dr_drill_runs') IS NOT NULL AND to_regclass('public.dr_drill_runs') IS NULL THEN
        EXECUTE 'ALTER TABLE disaster_recovery.dr_drill_runs SET SCHEMA public';
    END IF;
    IF to_regclass('disaster_recovery.dr_drill_evidence') IS NOT NULL AND to_regclass('public.dr_drill_evidence') IS NULL THEN
        EXECUTE 'ALTER TABLE disaster_recovery.dr_drill_evidence SET SCHEMA public';
    END IF;
    IF to_regclass('disaster_recovery.dr_restore_operations') IS NOT NULL AND to_regclass('public.dr_restore_operations') IS NULL THEN
        EXECUTE 'ALTER TABLE disaster_recovery.dr_restore_operations SET SCHEMA public';
    END IF;
    IF to_regclass('disaster_recovery.dr_breakglass_grants') IS NOT NULL AND to_regclass('public.dr_breakglass_grants') IS NULL THEN
        EXECUTE 'ALTER TABLE disaster_recovery.dr_breakglass_grants SET SCHEMA public';
    END IF;
END $$;
