-- migration:      20261128160000
-- description:    revert deployment_strategies schema and rollout execution ledger
-- author:         platform-core
-- date:           2026-11-28T16:00:00Z
-- reversible:     YES

SET lock_timeout = '3s';

DROP TABLE IF EXISTS deployment_strategies.processed_events CASCADE;
DROP TABLE IF EXISTS deployment_strategies.outbox CASCADE;
DROP TABLE IF EXISTS deployment_strategies.rollout_analysis_runs CASCADE;
DROP TABLE IF EXISTS deployment_strategies.rollout_executions CASCADE;
DROP TABLE IF EXISTS deployment_strategies.rollout_plans CASCADE;
DROP FUNCTION IF EXISTS deployment_strategies.forbid_immutable_rollout_columns() CASCADE;
DROP SCHEMA IF EXISTS deployment_strategies CASCADE;
