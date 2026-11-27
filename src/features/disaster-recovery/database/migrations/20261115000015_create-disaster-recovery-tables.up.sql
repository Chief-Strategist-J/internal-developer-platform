-- migration:      0015
-- description:    create disaster recovery plans and incidents tables with 05-db-writing-rules compliance
-- author:         platform-core
-- date:           2026-11-24T10:00:00Z
-- depends_on:     0014
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         persists dr plans, incidents, failover operations, and fencing epochs
-- entity:         dr_plans
-- contract_rev:   1.0
-- lock_timeout:   3s
-- recipe:         CHECK | FK | NOT NULL | UNIQUE | TRIGGER

SET lock_timeout = '3s';
SET statement_timeout = '5min';

CREATE TABLE IF NOT EXISTS dr_plan_status (
    code TEXT PRIMARY KEY,
    description TEXT NOT NULL
);

INSERT INTO dr_plan_status (code, description) VALUES
    ('NOT_READY', 'Plan dependencies failing readiness check'),
    ('READY', 'All regional dependencies verified healthy'),
    ('DEGRADED', 'Minor dependency warning, manual intervention needed'),
    ('DECLARED', 'Disaster declared by dual approval'),
    ('FENCING', 'Revoking primary write paths'),
    ('PROMOTING', 'Promoting DR database replica'),
    ('SCALING', 'Scaling DR compute and consuming reservations'),
    ('VALIDATING', 'Validating DR cluster workload health'),
    ('ROUTING', 'Switching DNS and load balancer traffic to DR'),
    ('FAILED_OVER', 'Failover complete and active in DR region'),
    ('RECONCILING', 'Reverse replication and divergence checking'),
    ('FAILBACK_READY', 'Zero divergence and ready for switchback'),
    ('FAILING_BACK', 'Switching back to primary region'),
    ('NORMAL', 'Restored to normal primary region operation'),
    ('ABORTED', 'Failover aborted prior to promotion')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS dr_topologies (
    code TEXT PRIMARY KEY,
    description TEXT NOT NULL
);

INSERT INTO dr_topologies (code, description) VALUES
    ('ACTIVE_ACTIVE', 'Simultaneous active multi-region traffic'),
    ('WARM_STANDBY', 'Scaled down replica with warm capacity'),
    ('PILOT_LIGHT', 'Core database replicating, compute scaled to zero'),
    ('RESTORE_TO_NEW', 'Backup restore into pre-created cluster')
ON CONFLICT (code) DO NOTHING;

CREATE TABLE IF NOT EXISTS dr_plans (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    public_id TEXT NOT NULL,
    tenant_id UUID NOT NULL,
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    correlation_id UUID NOT NULL DEFAULT gen_random_uuid(),
    plan_name TEXT NOT NULL,
    incident_tier TEXT NOT NULL DEFAULT 'TIER_0',
    primary_region TEXT NOT NULL DEFAULT 'us-central1',
    dr_region TEXT NOT NULL DEFAULT 'us-east4',
    topology TEXT NOT NULL DEFAULT 'WARM_STANDBY',
    target_rto_seconds INTEGER NOT NULL DEFAULT 300,
    target_rpo_seconds INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'NOT_READY',
    data_store_type TEXT NOT NULL DEFAULT 'CLOUD_SQL',
    secret_sync_source TEXT NOT NULL DEFAULT 'GOOGLE_SECRET_MANAGER',
    primary_cluster_id TEXT NOT NULL,
    dr_cluster_id TEXT NOT NULL,
    last_drill_at TIMESTAMPTZ,
    last_drill_result TEXT,
    idempotency_key TEXT NOT NULL,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT dr_plans_tenant_id_id_pk PRIMARY KEY (tenant_id, id),
    CONSTRAINT dr_plans_public_id_uk UNIQUE (public_id),
    CONSTRAINT dr_plans_tenant_idempotency_uk UNIQUE (tenant_id, idempotency_key),
    CONSTRAINT dr_plans_tenant_source_external_uk UNIQUE (tenant_id, source, external_id),
    CONSTRAINT dr_plans_status_fk FOREIGN KEY (status) REFERENCES dr_plan_status(code) ON DELETE RESTRICT,
    CONSTRAINT dr_plans_topology_fk FOREIGN KEY (topology) REFERENCES dr_topologies(code) ON DELETE RESTRICT,
    CONSTRAINT dr_plans_plan_name_ck CHECK (plan_name = btrim(plan_name) AND plan_name <> '' AND length(plan_name) <= 128),
    CONSTRAINT dr_plans_regions_distinct_ck CHECK (primary_region <> dr_region),
    CONSTRAINT dr_plans_incident_tier_ck CHECK (incident_tier IN ('TIER_0', 'TIER_1', 'TIER_2', 'TIER_3')),
    CONSTRAINT dr_plans_rto_nonneg_ck CHECK (target_rto_seconds >= 0),
    CONSTRAINT dr_plans_rpo_nonneg_ck CHECK (target_rpo_seconds >= 0),
    CONSTRAINT dr_plans_clusters_distinct_ck CHECK (primary_cluster_id <> dr_cluster_id)
);

CREATE TABLE IF NOT EXISTS dr_fencing_epochs (
    dataset_id TEXT PRIMARY KEY,
    authority TEXT NOT NULL,
    epoch BIGINT NOT NULL DEFAULT 1,
    fenced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT dr_fencing_dataset_ck CHECK (dataset_id = btrim(dataset_id) AND dataset_id <> ''),
    CONSTRAINT dr_fencing_authority_ck CHECK (authority IN ('PRIMARY', 'DR_REPLICA', 'FENCED_OFFLINE'))
);

