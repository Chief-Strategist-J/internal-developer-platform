SET lock_timeout = '3s';
SET statement_timeout = '5min';

DO $$
BEGIN
    IF to_regclass('policy_as_code.policy_lifecycle_states') IS NOT NULL AND to_regclass('public.policy_lifecycle_states') IS NULL THEN
        EXECUTE 'ALTER TABLE policy_as_code.policy_lifecycle_states SET SCHEMA public';
    END IF;
    IF to_regclass('policy_as_code.policy_state_transitions') IS NOT NULL AND to_regclass('public.policy_state_transitions') IS NULL THEN
        EXECUTE 'ALTER TABLE policy_as_code.policy_state_transitions SET SCHEMA public';
    END IF;
    IF to_regclass('policy_as_code.policy_exception_status') IS NOT NULL AND to_regclass('public.policy_exception_status') IS NULL THEN
        EXECUTE 'ALTER TABLE policy_as_code.policy_exception_status SET SCHEMA public';
    END IF;
    IF to_regclass('policy_as_code.policies') IS NOT NULL AND to_regclass('public.policies') IS NULL THEN
        EXECUTE 'ALTER TABLE policy_as_code.policies SET SCHEMA public';
    END IF;
    IF to_regclass('policy_as_code.policy_exceptions') IS NOT NULL AND to_regclass('public.policy_exceptions') IS NULL THEN
        EXECUTE 'ALTER TABLE policy_as_code.policy_exceptions SET SCHEMA public';
    END IF;
    IF to_regclass('policy_as_code.policy_violation_status') IS NOT NULL AND to_regclass('public.policy_violation_status') IS NULL THEN
        EXECUTE 'ALTER TABLE policy_as_code.policy_violation_status SET SCHEMA public';
    END IF;
    IF to_regclass('policy_as_code.policy_violations') IS NOT NULL AND to_regclass('public.policy_violations') IS NULL THEN
        EXECUTE 'ALTER TABLE policy_as_code.policy_violations SET SCHEMA public';
    END IF;
END $$;
