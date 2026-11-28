# Architecture Decision Records (ADRs) — Self-Service Provisioning

## ADR-001: Hexagonal Core Architecture & PostgreSQL Distributed Governance Ledger
- **Status:** Accepted & Fully Mechanized
- **Context:**
  Durable workflow engines (e.g. Temporal, Argo Workflows) retry failed or timed-out steps indefinitely by default. Concurrently, cloud infrastructure providers (GCP Config Connector, AWS Controllers for Kubernetes) assign server-generated resource IDs asynchronously. Under network drops or worker crashes, conventional name-based deduplication fails, causing duplicate resource creation, quota leaks, and split-brain states (`INV-01`, `INV-02`, `INV-03`, `INV-04`, `INV-09`, `INV-10`).
- **Decision:**
  Enforce a strictly mechanized PostgreSQL governance ledger in the `self_service_provisioning` schema:
  1. **Intent-Before-Effect (`INV-01`)**: No external side effect without a persisted intent row (`provisioning_intents`) storing request key, step name, attempt counter, and lookup labels.
  2. **Optimistic Fencing Tokens (`INV-03`)**: Every outcome write is fenced with `WHERE attempt = :mine`. Stale workers or paused containers cannot write back after lease expiration.
  3. **Atomic Multi-Resource Reservations (`INV-04`, `INV-05`, `INV-15`)**: Pre-allocate resource names, quota units, and IP CIDR blocks in a single ACID transaction (`provisioning_reservations`) before executing step 1.
  4. **Approval & Plan Pinning (`INV-09`, `INV-10`)**: Cryptographic binding of SHA-256 plan hashes (`provisioning_approvals`), preventing mid-flight drift of IaC templates or security policies.
  5. **Governance State Locks (`INV-06`, `INV-07`, `INV-13`)**: Dedicated ledger tables for retention decisions (`retention_decisions`), DNS cooldown tracking (`dns_cooldowns`), and mutex locks (`saga_locks`).
- **Consequences:**
  - Idempotent and crash-proof workflow progression.
  - Zero mock bypasses in production paths.
  - Verified via Go architecture tests (`tests/self-service-provisioning_test.go`) and database migration test suite.

---

## ADR-002: Adapter Decoupling & Isolation Boundary for External Cloud Providers (Invariant A)
- **Status:** Accepted & Fully Mechanized
- **Context:**
  Contract v4 Invariant A mandates strict physical and architectural decoupling: 1 file per hexagonal adapter, centralized query definitions in `queries/`, zero mid-function comments, and zero direct vendor coupling in domain logic.
- **Decision:**
  Isolate domain orchestration (`self-service-provisioning.machine.go` and `self-service-provisioning.saga.workflow.go`) behind discrete hexagonal ports:
  - `ProvisioningGovernancePort` → `self-service-provisioning.governance.adapter.go`
  - `ProvisioningReservationPort` → `self-service-provisioning.reservation.adapter.go`
  - `ProvisioningApprovalPort` → `self-service-provisioning.approval.adapter.go`
  - `ProvisioningIntentPort` → `self-service-provisioning.intent.adapter.go`
  - `ProvisioningSweeperPort` → `self-service-provisioning.sweeper.adapter.go`
  - `ClusterProvisionerPort` & `CloudResourceProvisionerPort` → decoupled infrastructure clients in `src/infra/clients/`
- **Consequences:**
  - Centralized raw SQL queries in `internal/infrastructure/queries/self-service-provisioning.queries.go`.
  - Compile-time interface verification ensuring zero leakage of cloud SDK types into domain rules.

---

## ADR-003: Distributed Leader Lease & Reconciler Architecture (Section C Sweeper Operating Model)
- **Status:** Accepted & Fully Mechanized
- **Context:**
  Section C mandates daily background sweeps to detect untracked resources, duplicate scan breaches, orphaned dirty states, and DNS ownership cooldown pruning. Running background reconcilers across horizontally scaled platform pods without coordination risks concurrent database lock contention, double compensation, and split-brain reconciliation.
- **Decision:**
  Implement a dual-track operational sweeper pattern:
  1. **In-Process Daemon (`kernel.Lifecycle`)**: `selfservice.sweeper` daemon running inside `src/main.go` that acquires a distributed Redis mutex lease (`ssp:lease:sweeper`, 5-minute TTL) using atomic `SetNX`. Only the elected leader executes the hourly sweep cycle (`RunSweeper`).
  2. **Declarative Kubernetes CronJob (`ssp-daily-sweeper`)**: Deployed via `deploy/k8s/base/apps/sweeper-cronjob.yaml`, running daily at 03:00 UTC to ping the health endpoint and verify that the distributed leader reconciler is active and healthy.
- **Consequences:**
  - Guaranteed single-leader execution without dedicated cluster coordinator pods.
  - Clean failover: If the leader pod crashes, the Redis lease expires within 300s, enabling another pod to assume leadership.

---

## ADR-004: Production Observability, Zero-Tolerance Alerts, and Grafana Telemetry Model
- **Status:** Accepted & Fully Mechanized
- **Context:**
  Section C defines hard SLIs, SLOs, and incident severities (PAGE vs TICKET). Platform operators require instant visibility into zero-tolerance breaches (`ORPHANED_DIRTY > 0`, duplicate scans, dangling DNS records, and stalled sagas).
- **Decision:**
  1. **Prometheus Alerting Architecture**:
     - Standard Prometheus alerting rules ConfigMap (`prometheus-rules`) deployed to `/etc/prometheus/rules/self-service-provisioning.rules.yaml`.
     - GitOps PrometheusRule CRD (`monitoring.coreos.com/v1`) for Prometheus Operator environments.
     - Hard separation into `PAGE` alerts (tier-1: `SSPOrphanedDirtyDetected`, `SSPDuplicateResourceDetected`, `SSPSagaStallDetected`, `SSPIPSpaceNearExhaustion`, `SSPDataClassDeletionWithoutDecision`, `SSPDanglingDNSDetected`, `SSPSecretsInRepositoryDetected`) and `TICKET` alerts (`SSPTenantQuotaDrift`, `SSPReadyWithoutReplicas`).
  2. **Grafana Dashboard Model (`ssp-operational-slos`)**:
     - Auto-provisioned dashboard JSON in `/var/lib/grafana/dashboards/self-service-provisioning.json`.
     - Structured into 4 operational rows: Incident & SLI Overview, Saga Funnel & Steps, Downstream Rate Limits (429/403/5xx), and Multi-Tenant Capacity/Quotas.
- **Consequences:**
  - Automated detection of breach conditions directly linked to operator runbooks in `failure-cases.md`.

---

## ADR-005: Distributed Redis Idempotency Middleware & Deterministic Response Replay
- **Status:** Accepted & Fully Mechanized
- **Context:**
  Client retries and duplicate HTTP submissions (`EC-02`) against provisioning endpoints must guarantee exactly-once execution. In-memory locks fail when requests hit different horizontally scaled API replicas.
- **Decision:**
  Replace volatile in-memory caching with Redis distributed persistence (`github.com/redis/go-redis/v9`):
  - In-flight distributed locking via atomic `SET idempotency:lock:<key> ... NX EX 30s`. Concurrent identical requests receive `409 Conflict: operation in-flight`.
  - Cached response replay via `idempotency:resp:<key>` with 24-hour TTL storing status code, headers, and payload SHA-256 hash.
  - Payload mismatch detection: Different body submitted with same idempotency key returns `422 Unprocessable Entity`.
  - Graceful degradation: Automatic fallback to local in-memory lock if Redis connection fails.
- **Consequences:**
  - Bulletproof idempotency across horizontal pod autoscaling.

---

## References & Architectural Standards
1. **Temporal Pre-Production Architecture & Activity Semantics (V-94)**:
   - Temporal documentation: *Activities execute at least once by default and must be strictly idempotent.* Unbounded retries without circuit breaking cause cascading failure storms. Abrupt worker kill drills validate at-least-once recovery.
2. **GitHub SCM Rate Limiting & Abuse Detection Guidance (V-95)**:
   - GitHub API v3/v4 Documentation: *Content creation endpoints are capped at 80 requests/minute and 500/hour.* Secondary rate limits provide no status endpoint; integrations must respect `Retry-After` / `X-RateLimit-Reset` headers or face IP blacklisting.
3. **Google Cloud Config Connector Resource Model (V-91)**:
   - Config Connector Architecture Guide: *Resources with server-assigned IDs can only be re-acquired via the `resourceID` field, not by spec name.* Lookups by request labels prior to creation prevent duplicate resource provisioning.
4. **GKE CIDR Allocation and Node Pool Subnet Constraints (V-92)**:
   - Google Kubernetes Engine Network Architecture: *A node's entire Pod CIDR is reserved regardless of current pod density.* Secondary Pod ranges cannot be retroactively assigned to existing node pools.
5. **Dangling DNS & Subdomain Takeover Countermeasures (V-93)**:
   - Cloud Security Alliance (CSA) Cloud Threat Defense: *Decommissioning cloud resources before releasing DNS CNAMEs creates high-risk subdomain takeover vulnerabilities.* Strict teardown ordering (DNS → Load Balancer → Backend) and cooldown quarantine are mandatory.
6. **RFC 7231 (HTTP/1.1 Semantics)** & **RFC 7807 (Problem Details for HTTP APIs)**:
   - Standardized error envelopes and idempotency key handling.
