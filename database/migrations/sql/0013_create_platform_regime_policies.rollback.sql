-- migration:      0013.rollback
-- description:    rollback platform regime policies table and lookup references
-- author:         chief-strategist
-- date:           2026-11-22T17:40:00Z
-- depends_on:     0012

DROP TRIGGER IF EXISTS trg_platform_regime_immutability ON platform_regime_policies;
DROP FUNCTION IF EXISTS trg_assert_platform_regime_immutable();
DROP TABLE IF EXISTS platform_regime_policies;
DROP TABLE IF EXISTS regime_policy_status;
DROP TABLE IF EXISTS regime_tenancy_modes;
DROP TABLE IF EXISTS regime_cluster_modes;
DROP TABLE IF EXISTS regime_kms_custody_modes;
