-- migration:      0005
-- description:    add strategy column and status check constraints to workload provisions
-- author:         chief-strategist
-- date:           2026-11-16T19:25:00Z
-- depends_on:     0004
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         align database schema with workload provisioning contracts and prevent invalid enum states

ALTER TABLE workload_provisions
    ADD COLUMN IF NOT EXISTS strategy VARCHAR(32) NOT NULL DEFAULT 'Canary';

ALTER TABLE workload_provisions
    DROP CONSTRAINT IF EXISTS chk_workload_strategy,
    ADD CONSTRAINT chk_workload_strategy CHECK (strategy IN ('Canary', 'RollingUpdate', 'Recreate'));

ALTER TABLE workload_provisions
    DROP CONSTRAINT IF EXISTS chk_workload_status,
    ADD CONSTRAINT chk_workload_status CHECK (status IN ('SUBMITTED', 'VALIDATED', 'ADMITTED', 'PROVISIONING', 'ACTIVE', 'OBSERVED_READY', 'DEGRADED', 'REJECTED'));
