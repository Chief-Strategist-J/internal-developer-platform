# OpenAPI Changelog

## [1.1.0] - 2026-11-23
### Added
- `POST /api/v1/regimes`: Endpoint to register and activate tenant compliance regime policies covering B-1 (Jurisdiction, Retention >= 30, Data Residency, KMS Key Custody), B-2 (RTO/RPO limits), B-3 (GKE Standard/Autopilot cluster mode and region fleet counts), and B-4 (Tenancy isolation mode).
- `GET /api/v1/regimes/active`: Endpoint to fetch the active platform regime policy for a given tenant ID with platform fallback.

