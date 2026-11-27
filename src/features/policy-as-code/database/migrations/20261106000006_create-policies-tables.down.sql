SET lock_timeout = '3s';
SET statement_timeout = '5min';

-- migration:      0006
-- description:    rollback policy tables
-- author:         platform-security
-- date:           2026-11-18T10:35:00Z
-- depends_on:     0005
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         clean rollback of policy-as-code tables

DROP TABLE IF EXISTS policy_exceptions;
DROP TABLE IF EXISTS policies;
DROP TABLE IF EXISTS policy_exception_status;
DROP TABLE IF EXISTS policy_state_transitions;
DROP TABLE IF EXISTS policy_lifecycle_states;
