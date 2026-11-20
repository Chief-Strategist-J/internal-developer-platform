-- rollback:       0009
-- description:    drop image supply chain columns from workload_provisions

ALTER TABLE workload_provisions
    DROP COLUMN IF EXISTS breakglass_justification,
    DROP COLUMN IF EXISTS image_pull_policy;
