-- migration:      0012
-- description:    rollback on-call paging fields and incident timeline events table
-- author:         chief-strategist
-- date:           2026-11-20T18:10:00Z
-- reversible:     YES

DROP TABLE IF EXISTS incident_timeline_events CASCADE;

ALTER TABLE workload_provisions
    DROP COLUMN IF EXISTS on_call_pager,
    DROP COLUMN IF EXISTS runbook_url,
    DROP COLUMN IF EXISTS incident_tier;

