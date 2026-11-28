/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PLATFORM REGIME SQL QUERIES

1. Parameterized Queries:
  - Super-fast, strictly parameterized queries matching 0013_create_platform_regime_policies.sql.
  - Zero inline comments inside function bodies.
*/
package queries

const (
	FlowInsertRegimePolicy = `
INSERT INTO platform_regime.platform_regime_policies (
    id, public_id, tenant_id, source, external_id, correlation_id,
    jurisdiction, audit_retention_days, data_residency_regions, kms_key_custody_mode,
    rto_minutes_tier0, rpo_minutes_tier0, rto_minutes_tier1, rpo_minutes_tier1,
    cluster_mode, primary_region_count, standby_region_count, tenant_isolation_mode,
    status, version, created_at, updated_at
) VALUES (
    $1, $2, $3, $4, $5, $6,
    $7, $8, $9, $10,
    $11, $12, $13, $14,
    $15, $16, $17, $18,
    $19, $20, $21, $22
) RETURNING id;`

	FlowSupersedePreviousActive = `
UPDATE platform_regime.platform_regime_policies
SET status = 'SUPERSEDED', updated_at = NOW()
WHERE tenant_id = $1 AND status = 'ACTIVE' AND id != $2;`

	FlowSelectActiveRegime = `
SELECT
    id, public_id, tenant_id, source, external_id, correlation_id,
    jurisdiction, audit_retention_days, data_residency_regions, kms_key_custody_mode,
    rto_minutes_tier0, rpo_minutes_tier0, rto_minutes_tier1, rpo_minutes_tier1,
    cluster_mode, primary_region_count, standby_region_count, tenant_isolation_mode,
    status, version, created_at, updated_at
FROM platform_regime.platform_regime_policies
WHERE tenant_id = $1 AND status = 'ACTIVE'
ORDER BY created_at DESC
LIMIT 1;`

	FlowSelectRegimeByID = `
SELECT
    id, public_id, tenant_id, source, external_id, correlation_id,
    jurisdiction, audit_retention_days, data_residency_regions, kms_key_custody_mode,
    rto_minutes_tier0, rpo_minutes_tier0, rto_minutes_tier1, rpo_minutes_tier1,
    cluster_mode, primary_region_count, standby_region_count, tenant_isolation_mode,
    status, version, created_at, updated_at
FROM platform_regime.platform_regime_policies
WHERE tenant_id = $1 AND id = $2;`
)
