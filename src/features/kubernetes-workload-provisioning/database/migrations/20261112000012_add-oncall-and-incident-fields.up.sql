SET lock_timeout = '3s';
SET statement_timeout = '5min';

-- migration:      0012
-- description:    add on-call paging, runbook url, incident tier, and incident timeline events table
-- author:         chief-strategist
-- date:           2026-11-20T18:10:00Z
-- depends_on:     0011
-- reversible:     YES
-- lock_risk:      LOW
-- rows_affected:  schema only
-- reason:         enforce 4.10 on-call & incidents compliance floor (24/7 paging, no runbook no page)

ALTER TABLE workload_provisions
    ADD COLUMN IF NOT EXISTS on_call_pager VARCHAR(255) DEFAULT '',
    ADD COLUMN IF NOT EXISTS runbook_url VARCHAR(512) DEFAULT '',
    ADD COLUMN IF NOT EXISTS incident_tier VARCHAR(32) DEFAULT 'TIER_2';

UPDATE workload_provisions
SET
    on_call_pager = COALESCE(on_call_pager, ''),
    runbook_url = COALESCE(runbook_url, ''),
    incident_tier = COALESCE(incident_tier, 'TIER_2')
WHERE on_call_pager IS NULL OR runbook_url IS NULL OR incident_tier IS NULL;

CREATE TABLE IF NOT EXISTS incident_timeline_events (
    id VARCHAR(64) PRIMARY KEY,
    tenant_id VARCHAR(64) NOT NULL,
    workload_id VARCHAR(64) NOT NULL,
    event_type VARCHAR(64) NOT NULL,
    description TEXT NOT NULL,
    actor VARCHAR(128) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_incident_events_workload ON incident_timeline_events (tenant_id, workload_id, created_at DESC);

