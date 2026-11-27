-- Phase 5 Expand-Safe Table Ownership Relocation into gitops_reconciliation (D-01, D-02)
SET lock_timeout = '3s';
SET statement_timeout = '5min';

DO $$
BEGIN
    IF to_regclass('public.gitops_reconciliations') IS NOT NULL AND to_regclass('gitops_reconciliation.gitops_reconciliations') IS NULL THEN
        EXECUTE 'ALTER TABLE public.gitops_reconciliations SET SCHEMA gitops_reconciliation';
    END IF;
    IF to_regclass('public.gitops_secret_mechanisms') IS NOT NULL AND to_regclass('gitops_reconciliation.gitops_secret_mechanisms') IS NULL THEN
        EXECUTE 'ALTER TABLE public.gitops_secret_mechanisms SET SCHEMA gitops_reconciliation';
    END IF;
    IF to_regclass('public.gitops_outbox_events') IS NOT NULL AND to_regclass('gitops_reconciliation.gitops_outbox_events') IS NULL THEN
        EXECUTE 'ALTER TABLE public.gitops_outbox_events SET SCHEMA gitops_reconciliation';
    END IF;
END $$;

GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA gitops_reconciliation TO svc_gitops_reconciliation;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA gitops_reconciliation TO svc_gitops_reconciliation;
