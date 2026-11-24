-- migration:      0016
-- description:    create dr incidents, operations, drills, and evidence tables
-- author:         platform-core
-- date:           2026-11-24T12:00:00Z
-- depends_on:     0015
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         persists dr incidents, failover operations, drill runs, and immutable evidence
-- entity:         dr_drill_evidence
-- contract_rev:   1.0
-- lock_timeout:   3s
-- recipe:         CHECK | FK | NOT NULL | UNIQUE

SET lock_timeout = '3s';

CREATE TABLE IF NOT EXISTS dr_incidents (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    public_id TEXT NOT NULL,
    tenant_id UUID NOT NULL,
    plan_id UUID NOT NULL,
    scope TEXT NOT NULL,
    declared_by TEXT NOT NULL,
    approved_by TEXT,
    evidence_links JSONB NOT NULL DEFAULT '[]'::jsonb,
    state TEXT NOT NULL DEFAULT 'PENDING_APPROVAL',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT dr_incidents_pk PRIMARY KEY (tenant_id, id),
    CONSTRAINT dr_incidents_public_id_uk UNIQUE (public_id),
    CONSTRAINT dr_incidents_scope_ck CHECK (scope IN ('ZONE', 'REGION', 'PROVIDER_GLOBAL', 'DATA_CORRUPTION', 'SECURITY_COMPROMISE')),
    CONSTRAINT dr_incidents_state_ck CHECK (state IN ('PENDING_APPROVAL', 'DECLARED', 'CANCELLED', 'RESOLVED'))
);

CREATE TABLE IF NOT EXISTS dr_failover_operations (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    public_id TEXT NOT NULL,
    tenant_id UUID NOT NULL,
    plan_id UUID NOT NULL,
    incident_id UUID NOT NULL,
    mode TEXT NOT NULL,
    initiator TEXT NOT NULL,
    approver TEXT,
    state TEXT NOT NULL,
    projected_rpo_seconds INTEGER NOT NULL DEFAULT 0,
    measured_rto_seconds INTEGER NOT NULL DEFAULT 0,
    step_results JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT dr_failover_ops_pk PRIMARY KEY (tenant_id, id),
    CONSTRAINT dr_failover_ops_public_id_uk UNIQUE (public_id),
    CONSTRAINT dr_failover_ops_mode_ck CHECK (mode IN ('SWITCHOVER', 'REPLICA_FAILOVER'))
);

CREATE TABLE IF NOT EXISTS dr_drill_runs (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    public_id TEXT NOT NULL,
    tenant_id UUID NOT NULL,
    plan_id UUID NOT NULL,
    scope TEXT NOT NULL,
    measured_rto_seconds INTEGER NOT NULL DEFAULT 0,
    measured_rpo_seconds INTEGER NOT NULL DEFAULT 0,
    target_rto_seconds INTEGER NOT NULL DEFAULT 300,
    target_rpo_seconds INTEGER NOT NULL DEFAULT 0,
    passed BOOLEAN NOT NULL DEFAULT true,
    evidence_bundle_id TEXT NOT NULL,
    executed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT dr_drill_runs_pk PRIMARY KEY (tenant_id, id),
    CONSTRAINT dr_drill_runs_public_id_uk UNIQUE (public_id)
);

CREATE TABLE IF NOT EXISTS dr_drill_evidence (
    id TEXT PRIMARY KEY,
    public_id TEXT NOT NULL,
    tenant_id UUID NOT NULL,
    drill_run_id UUID NOT NULL,
    plan_id UUID NOT NULL,
    executed_by TEXT NOT NULL,
    scope TEXT NOT NULL,
    measured_rto_seconds INTEGER NOT NULL DEFAULT 0,
    measured_rpo_seconds INTEGER NOT NULL DEFAULT 0,
    passed BOOLEAN NOT NULL DEFAULT true,
    verification_logs JSONB NOT NULL DEFAULT '[]'::jsonb,
    audit_sink_region TEXT NOT NULL,
    checksum_sha256 TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT dr_drill_evidence_public_id_uk UNIQUE (public_id)
);

CREATE TABLE IF NOT EXISTS dr_restore_operations (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    public_id TEXT NOT NULL,
    tenant_id UUID NOT NULL,
    plan_id UUID NOT NULL,
    backup_id TEXT NOT NULL,
    target_cluster TEXT NOT NULL,
    namespace TEXT NOT NULL,
    state TEXT NOT NULL,
    data_validated BOOLEAN NOT NULL DEFAULT false,
    validation_note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT dr_restore_ops_pk PRIMARY KEY (tenant_id, id),
    CONSTRAINT dr_restore_ops_public_id_uk UNIQUE (public_id)
);

CREATE TABLE IF NOT EXISTS dr_breakglass_grants (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    public_id TEXT NOT NULL,
    tenant_id UUID NOT NULL,
    requester TEXT NOT NULL,
    approver TEXT NOT NULL,
    role TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    justification TEXT NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT dr_breakglass_pk PRIMARY KEY (tenant_id, id),
    CONSTRAINT dr_breakglass_public_id_uk UNIQUE (public_id),
    CONSTRAINT dr_breakglass_distinct_actors_ck CHECK (requester <> approver)
);

