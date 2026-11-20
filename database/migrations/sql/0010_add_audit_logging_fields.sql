-- migration:      0010
-- description:    add audit logging and log sink columns to workload_provisions
-- author:         chief-strategist
-- date:           2026-11-20T15:40:00Z
-- depends_on:     0009
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         enforce observe and audit compliance floor (V-32, V-23) per business-logic.md

ALTER TABLE workload_provisions
    ADD COLUMN IF NOT EXISTS enable_data_access_logs BOOLEAN DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS audit_destination_bucket VARCHAR(255) DEFAULT '',
    ADD COLUMN IF NOT EXISTS audit_retention_days INT DEFAULT 30,
    ADD COLUMN IF NOT EXISTS audit_locked_retention BOOLEAN DEFAULT FALSE;

UPDATE workload_provisions
SET
    enable_data_access_logs = COALESCE(enable_data_access_logs, FALSE),
    audit_destination_bucket = COALESCE(audit_destination_bucket, ''),
    audit_retention_days = COALESCE(audit_retention_days, 30),
    audit_locked_retention = COALESCE(audit_locked_retention, FALSE)
WHERE audit_destination_bucket IS NULL OR audit_retention_days IS NULL;

