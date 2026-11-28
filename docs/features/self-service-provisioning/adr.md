# Architecture Decision Records — Self-Service Provisioning

## ADR-001: Hexagonal Core + PostgreSQL Governance Ledger
- **Status:** Accepted (Implemented in `self-service-provisioning`)
- **Context:** Durable workflow retries and server-assigned cloud IDs defeat name-only deduplication (`INV-01`..`INV-04`, `INV-09`, `INV-10`).
- **Decision:** Enforce intent-before-effect (`provisioning_intents`), fenced attempt CAS (`WHERE attempt = :mine`), atomic name/quota/IP reservations (`provisioning_reservations`), approval pinning (`provisioning_approvals`), and governance locks/cooldowns (`retention_decisions`, `dns_cooldowns`, `saga_locks`) inside the `self_service_provisioning` PostgreSQL schema.
- **Consequences:** All invariants independent of external workflow/IaC drivers are enforced and unit-tested; live Postgres verification runs via `scripts/migrate/migrate.sh` and `scripts/test/test.curl-api.sh`.

## ADR-002: Open Platform Decisions (Blocking External Drivers)
- **Status:** Proposed / Pending Decision
- **Open Blockers:** `D-WF` (durable workflow engine, e.g. Temporal), `D-IAC` (Config Connector / Terraform / Crossplane), `D-SCM` (GitHub / GitLab), `D-CI`, `D-TPL`, `D-BUDGET`, `B-1`.
- **Decision:** Keep external orchestration behind `ProvisioningGovernancePort`, `ClusterProvisionerPort`, `CloudResourceProvisionerPort`, and `src/infra/clients/{argocd,gitops}` so no mock/fake production drivers are introduced (`G-00`).
