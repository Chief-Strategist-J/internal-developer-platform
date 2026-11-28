# OpenAPI Changelog

## [1.2.0] - 2026-11-28
### Added
- `POST /api/v1/autoscaling/plans:evaluate`: Evaluates a scaling plan against autoscaling invariants AS-INV-01..15 and returns every violation. 64-bit quantities are string-serialized; tenant comes from `x-tenant-id` only. Covers AS-INV-01..23 (adds scale-to-zero safety and ingestion signal/dead-letter rules; surge and canary pods count toward the ceilings chain; tier floors apply only in production) including database connection ceiling, governed DB scaling, storage bounds, Kafka partition capping, Pub/Sub consumer floor and transaction-pooler compatibility.

## [1.1.0] - 2026-11-23
### Added
- `POST /api/v1/regimes`: Endpoint to register and activate tenant compliance regime policies covering B-1 (Jurisdiction, Retention >= 30, Data Residency, KMS Key Custody), B-2 (RTO/RPO limits), B-3 (GKE Standard/Autopilot cluster mode and region fleet counts), and B-4 (Tenancy isolation mode).
- `GET /api/v1/regimes/active`: Endpoint to fetch the active platform regime policy for a given tenant ID with platform fallback.

