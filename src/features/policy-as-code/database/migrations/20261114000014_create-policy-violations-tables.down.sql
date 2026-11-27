-- migration:      0014.rollback
-- description:    rollback policy violations table and lookup references
-- author:         platform-security
-- date:           2026-11-23T15:45:00Z
-- depends_on:     0013
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         reverts policy violations table and status references
-- entity:         policy_violations
-- contract_rev:   1.0
-- lock_timeout:   3s
-- recipe:         TRIGGER | TABLE

SET lock_timeout = '3s';

DROP TRIGGER IF EXISTS trg_policy_violations_immutability ON policy_violations;
DROP FUNCTION IF EXISTS trg_assert_policy_violations_immutable();
DROP TABLE IF EXISTS policy_violations;
DROP TABLE IF EXISTS policy_violation_status;
