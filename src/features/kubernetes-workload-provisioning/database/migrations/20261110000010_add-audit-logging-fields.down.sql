SET lock_timeout = '3s';
SET statement_timeout = '5min';

ALTER TABLE workload_provisions
    DROP COLUMN IF EXISTS enable_data_access_logs,
    DROP COLUMN IF EXISTS audit_destination_bucket,
    DROP COLUMN IF EXISTS audit_retention_days,
    DROP COLUMN IF EXISTS audit_locked_retention;

