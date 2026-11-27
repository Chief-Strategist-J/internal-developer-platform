-- rollback:    0016
-- description: revert dr incidents, operations, drills, and evidence tables
-- date:        2026-11-24T12:00:00Z

SET lock_timeout = '3s';

DROP TABLE IF EXISTS dr_breakglass_grants CASCADE;
DROP TABLE IF EXISTS dr_restore_operations CASCADE;
DROP TABLE IF EXISTS dr_drill_evidence CASCADE;
DROP TABLE IF EXISTS dr_drill_runs CASCADE;
DROP TABLE IF EXISTS dr_failover_operations CASCADE;
DROP TABLE IF EXISTS dr_incidents CASCADE;

