SET lock_timeout = '3s';
SET statement_timeout = '5min';

ALTER TABLE workload_provisions
    DROP CONSTRAINT IF EXISTS chk_workload_status,
    DROP CONSTRAINT IF EXISTS chk_workload_strategy,
    DROP COLUMN IF EXISTS strategy;
