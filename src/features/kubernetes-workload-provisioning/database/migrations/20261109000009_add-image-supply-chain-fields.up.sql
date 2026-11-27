SET lock_timeout = '3s';
SET statement_timeout = '5min';

-- migration:      0009
-- description:    add image pull policy and breakglass justification columns to workload_provisions
-- author:         chief-strategist
-- date:           2026-11-20T12:55:00Z
-- depends_on:     0008
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         enforce image supply chain floor (V-20, V-21, V-29) per business-logic.md

ALTER TABLE workload_provisions
    ADD COLUMN IF NOT EXISTS image_pull_policy VARCHAR(32) DEFAULT 'IfNotPresent',
    ADD COLUMN IF NOT EXISTS breakglass_justification TEXT DEFAULT '';

UPDATE workload_provisions
SET
    image_pull_policy = COALESCE(NULLIF(image_pull_policy, ''), 'IfNotPresent'),
    breakglass_justification = COALESCE(breakglass_justification, '')
WHERE image_pull_policy = '' OR breakglass_justification IS NULL;

