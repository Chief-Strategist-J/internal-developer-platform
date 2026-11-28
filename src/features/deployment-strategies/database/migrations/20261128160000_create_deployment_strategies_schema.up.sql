-- migration:      20261128160000
-- description:    create deployment_strategies schema and rollout execution ledger per Data Contract v1.2
-- author:         platform-core
-- date:           2026-11-28T16:00:00Z
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         implement durable rollout plans, executions, and analysis run history (P0-01..P0-08)
-- entity:         deployment_strategies
-- contract_rev:   1.2
-- lock_timeout:   3s
-- recipe:         SCHEMA | TABLE | CHECK | TRIGGER | RLS

SET lock_timeout = '3s';
SET statement_timeout = '5min';

CREATE SCHEMA IF NOT EXISTS deployment_strategies;

CREATE TABLE IF NOT EXISTS deployment_strategies.rollout_plans (
    id VARCHAR(128) PRIMARY KEY CHECK (id <> ''),
    public_id VARCHAR(128) NOT NULL UNIQUE,
    tenant_id VARCHAR(128) NOT NULL,
    name VARCHAR(128) NOT NULL,
    strategy_type VARCHAR(32) NOT NULL,
    steps JSONB NOT NULL,
    failure_threshold NUMERIC(5,2) NOT NULL DEFAULT 1.00,
    correlation_id UUID NOT NULL DEFAULT gen_random_uuid(),
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ds_plans_tenant_id_uq UNIQUE (tenant_id, id),
    CONSTRAINT ds_plans_public_id_fmt_ck CHECK (public_id ~ '^dplan_[0-9a-f]{26}$'),
    CONSTRAINT ds_plans_tenant_trimmed_ck CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
    CONSTRAINT ds_plans_name_trimmed_ck CHECK (name <> '' AND name = btrim(name)),
    CONSTRAINT ds_plans_strategy_type_ck CHECK (strategy_type IN ('CANARY', 'BLUE_GREEN', 'PROGRESSIVE_MESH')),
    CONSTRAINT ds_plans_steps_json_ck CHECK (jsonb_typeof(steps) = 'array'),
    CONSTRAINT ds_plans_failure_thresh_ck CHECK (failure_threshold >= 0 AND failure_threshold <= 100),
    CONSTRAINT ds_plans_version_pos_ck CHECK (version >= 1)
);

CREATE INDEX IF NOT EXISTS idx_ds_plans_tenant_strategy
    ON deployment_strategies.rollout_plans (tenant_id, strategy_type);

CREATE TABLE IF NOT EXISTS deployment_strategies.rollout_executions (
    id VARCHAR(128) PRIMARY KEY CHECK (id <> ''),
    public_id VARCHAR(128) NOT NULL UNIQUE,
    tenant_id VARCHAR(128) NOT NULL,
    plan_id VARCHAR(128) NOT NULL,
    workload_id VARCHAR(128) NOT NULL,
    target_image VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL,
    current_step_index INTEGER NOT NULL DEFAULT 0,
    current_weight_pct INTEGER NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    correlation_id UUID NOT NULL DEFAULT gen_random_uuid(),
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ds_exec_tenant_id_uq UNIQUE (tenant_id, id),
    CONSTRAINT ds_exec_plan_fk FOREIGN KEY (plan_id) REFERENCES deployment_strategies.rollout_plans(id) ON DELETE RESTRICT,
    CONSTRAINT ds_exec_public_id_fmt_ck CHECK (public_id ~ '^dexec_[0-9a-f]{26}$'),
    CONSTRAINT ds_exec_tenant_trimmed_ck CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
    CONSTRAINT ds_exec_workload_trimmed_ck CHECK (workload_id <> '' AND workload_id = btrim(workload_id)),
    CONSTRAINT ds_exec_target_image_trimmed_ck CHECK (target_image <> '' AND target_image = btrim(target_image)),
    CONSTRAINT ds_exec_status_ck CHECK (status IN ('INITIALIZING', 'ROUTING_STEP', 'ANALYZING', 'PAUSED', 'PROMOTING', 'COMPLETED', 'ABORTING', 'ABORTED')),
    CONSTRAINT ds_exec_step_index_ck CHECK (current_step_index >= 0),
    CONSTRAINT ds_exec_weight_pct_ck CHECK (current_weight_pct >= 0 AND current_weight_pct <= 100),
    CONSTRAINT ds_exec_version_pos_ck CHECK (version >= 1)
);

CREATE INDEX IF NOT EXISTS idx_ds_exec_tenant_workload
    ON deployment_strategies.rollout_executions (tenant_id, workload_id, status);

CREATE TABLE IF NOT EXISTS deployment_strategies.rollout_analysis_runs (
    id VARCHAR(128) PRIMARY KEY CHECK (id <> ''),
    public_id VARCHAR(128) NOT NULL UNIQUE,
    tenant_id VARCHAR(128) NOT NULL,
    execution_id VARCHAR(128) NOT NULL,
    metric_name VARCHAR(64) NOT NULL,
    query TEXT NOT NULL,
    measured_value NUMERIC(10,4) NOT NULL,
    threshold_value NUMERIC(10,4) NOT NULL,
    verdict VARCHAR(16) NOT NULL,
    correlation_id UUID NOT NULL DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ds_analysis_tenant_id_uq UNIQUE (tenant_id, id),
    CONSTRAINT ds_analysis_exec_fk FOREIGN KEY (execution_id) REFERENCES deployment_strategies.rollout_executions(id) ON DELETE RESTRICT,
    CONSTRAINT ds_analysis_public_id_fmt_ck CHECK (public_id ~ '^danly_[0-9a-f]{26}$'),
    CONSTRAINT ds_analysis_tenant_trimmed_ck CHECK (tenant_id <> '' AND tenant_id = btrim(tenant_id)),
    CONSTRAINT ds_analysis_metric_trimmed_ck CHECK (metric_name <> '' AND metric_name = btrim(metric_name)),
    CONSTRAINT ds_analysis_query_not_empty_ck CHECK (query <> ''),
    CONSTRAINT ds_analysis_verdict_ck CHECK (verdict IN ('PASS', 'FAIL', 'INCONCLUSIVE'))
);

CREATE INDEX IF NOT EXISTS idx_ds_analysis_exec
    ON deployment_strategies.rollout_analysis_runs (execution_id, created_at);

CREATE TABLE IF NOT EXISTS deployment_strategies.outbox (
    id VARCHAR(128) PRIMARY KEY CHECK (id <> ''),
    aggregate_id VARCHAR(128) NOT NULL CHECK (aggregate_id <> ''),
    event_type VARCHAR(128) NOT NULL CHECK (event_type <> ''),
    payload JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS deployment_strategies.processed_events (
    event_id VARCHAR(128) PRIMARY KEY CHECK (event_id <> ''),
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE OR REPLACE FUNCTION deployment_strategies.forbid_immutable_rollout_columns()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog
AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'id is immutable';
    END IF;
    IF NEW.public_id IS DISTINCT FROM OLD.public_id THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'public_id is immutable';
    END IF;
    IF NEW.tenant_id IS DISTINCT FROM OLD.tenant_id THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'tenant_id is immutable';
    END IF;
    IF NEW.correlation_id IS DISTINCT FROM OLD.correlation_id THEN
        RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'correlation_id is immutable';
    END IF;
    RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS ds_plans_immutable_trg ON deployment_strategies.rollout_plans;
CREATE TRIGGER ds_plans_immutable_trg
    BEFORE UPDATE ON deployment_strategies.rollout_plans
    FOR EACH ROW EXECUTE FUNCTION deployment_strategies.forbid_immutable_rollout_columns();

DROP TRIGGER IF EXISTS ds_exec_immutable_trg ON deployment_strategies.rollout_executions;
CREATE TRIGGER ds_exec_immutable_trg
    BEFORE UPDATE ON deployment_strategies.rollout_executions
    FOR EACH ROW EXECUTE FUNCTION deployment_strategies.forbid_immutable_rollout_columns();

ALTER TABLE deployment_strategies.rollout_plans ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS ds_plans_tenant_isolation ON deployment_strategies.rollout_plans;
CREATE POLICY ds_plans_tenant_isolation ON deployment_strategies.rollout_plans
    USING (
        current_setting('app.current_tenant_id', true) IS NULL
        OR current_setting('app.current_tenant_id', true) = ''
        OR tenant_id = current_setting('app.current_tenant_id', true)
    );

ALTER TABLE deployment_strategies.rollout_executions ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS ds_exec_tenant_isolation ON deployment_strategies.rollout_executions;
CREATE POLICY ds_exec_tenant_isolation ON deployment_strategies.rollout_executions
    USING (
        current_setting('app.current_tenant_id', true) IS NULL
        OR current_setting('app.current_tenant_id', true) = ''
        OR tenant_id = current_setting('app.current_tenant_id', true)
    );

ALTER TABLE deployment_strategies.rollout_analysis_runs ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS ds_analysis_tenant_isolation ON deployment_strategies.rollout_analysis_runs;
CREATE POLICY ds_analysis_tenant_isolation ON deployment_strategies.rollout_analysis_runs
    USING (
        current_setting('app.current_tenant_id', true) IS NULL
        OR current_setting('app.current_tenant_id', true) = ''
        OR tenant_id = current_setting('app.current_tenant_id', true)
    );
