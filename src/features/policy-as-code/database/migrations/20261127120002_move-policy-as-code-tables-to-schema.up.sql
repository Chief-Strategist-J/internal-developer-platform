-- Phase 5 Expand-Safe Table Ownership Relocation into policy_as_code (D-01, D-02)
SET lock_timeout = '3s';
SET statement_timeout = '5min';

DO $$
BEGIN
    IF to_regclass('public.policy_lifecycle_states') IS NOT NULL AND to_regclass('policy_as_code.policy_lifecycle_states') IS NULL THEN
        EXECUTE 'ALTER TABLE public.policy_lifecycle_states SET SCHEMA policy_as_code';
    END IF;
    IF to_regclass('public.policy_state_transitions') IS NOT NULL AND to_regclass('policy_as_code.policy_state_transitions') IS NULL THEN
        EXECUTE 'ALTER TABLE public.policy_state_transitions SET SCHEMA policy_as_code';
    END IF;
    IF to_regclass('public.policy_exception_status') IS NOT NULL AND to_regclass('policy_as_code.policy_exception_status') IS NULL THEN
        EXECUTE 'ALTER TABLE public.policy_exception_status SET SCHEMA policy_as_code';
    END IF;
    IF to_regclass('public.policies') IS NOT NULL AND to_regclass('policy_as_code.policies') IS NULL THEN
        EXECUTE 'ALTER TABLE public.policies SET SCHEMA policy_as_code';
    END IF;
    IF to_regclass('public.policy_exceptions') IS NOT NULL AND to_regclass('policy_as_code.policy_exceptions') IS NULL THEN
        EXECUTE 'ALTER TABLE public.policy_exceptions SET SCHEMA policy_as_code';
    END IF;
    IF to_regclass('public.policy_violation_status') IS NOT NULL AND to_regclass('policy_as_code.policy_violation_status') IS NULL THEN
        EXECUTE 'ALTER TABLE public.policy_violation_status SET SCHEMA policy_as_code';
    END IF;
    IF to_regclass('public.policy_violations') IS NOT NULL AND to_regclass('policy_as_code.policy_violations') IS NULL THEN
        EXECUTE 'ALTER TABLE public.policy_violations SET SCHEMA policy_as_code';
    END IF;
END $$;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA policy_as_code TO svc_policy_as_code;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA policy_as_code TO svc_policy_as_code;
