SET lock_timeout = '3s';
SET statement_timeout = '5min';

-- migration:      0011
-- description:    rollback access_grants table
-- author:         chief-strategist
-- date:           2026-11-20T17:45:00Z
-- reversible:     YES

DROP TABLE IF EXISTS access_grants CASCADE;

