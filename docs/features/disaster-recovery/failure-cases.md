# Disaster Recovery: Critical Failure Points (CFP-01 to CFP-20) & Edge Cases

This document details the exact mitigation, detection, and enforcement mechanisms for the 20 Critical Failure Points and Edge Cases defined in `business-logic.md`.

---

## 1. Twenty Critical Failure Points

### CFP-01: Provider Control Plane Down Globally [OUTAGE]
- **Scenario**: Global cloud authorization incident (e.g. June 12, 2025 Google Cloud Service Control crash) prevents all API mutations across every region.
- **Impact**: Dynamic failover scripts calling cloud provisioning APIs fail.
- **Handling**: Enforce **static stability** (ADR-001). DR clusters, regional DNS routing policies, and node capacity reservations are pre-created. Failover never requires creating new infrastructure in the outage window.
- **Detection**: Pre-flight readiness probes; synthetic drill verifying zero cloud mutations during cutover.

### CFP-02: Split-Brain After Primary Region Returns [CORRUPT]
- **Scenario**: Primary region recovers while DR is serving traffic; clients resume writes to old primary.
- **Impact**: Dual-writer data corruption and divergent transaction logs.
- **Handling**: **DR-INV-01 (Single Write Authority)**. Fencing epoch recorded in `dr_fencing_epochs`. Old primary IAM write roles and VPC write routes are revoked before replica promotion.
- **Detection**: Continuous write-endpoint resolution audit; alerting if $> 1$ write authority detected.

### CFP-03: Data Loss on Replica Failover [LOSS]
- **Scenario**: Regional outage forces promote-replica under positive replication lag.
- **Impact**: Unreplicated transactions in flight are lost.
- **Handling**: Control API surfaces live projected RPO via `GET /api/v1/dr/plans/{plan}/rpo`. Approvers must explicitly acknowledge data-loss window; Tier-0 mandates switchover mode (RPO = 0).
- **Detection**: Replication lag SLI gauges; PAGE alert when lag exceeds tier threshold.

### CFP-04: Automatic Failback Before Reconciliation [CORRUPT]
- **Scenario**: Health checks recover on primary, causing Cloud DNS routing policies to fail back automatically.
- **Impact**: In-flight writes in DR are severed; primary database lacks DR transactions.
- **Handling**: **DR-INV-02 (Locked Routing Policy)**. Stateful DNS endpoints use locked routing (`LockRoutingPolicy`). Failback requires explicit API invocation (`POST /api/v1/dr/failbacks`).
- **Detection**: Cloud DNS routing policy diff monitor; alert on unauthorized route change during active DR.

### CFP-05: Encryption Key Unavailable or Destroyed [LOSS]
- **Scenario**: CMEK key protecting DR backups is disabled or scheduled for destruction.
- **Impact**: Backups and encrypted disks become permanently unreadable.
- **Handling**: Sovereign DR CMEK keys pre-created in DR region (B-1); destruction protection enabled.
- **Detection**: Key-state monitoring probe; restore drill verifies end-to-end cryptographic decryption.

### CFP-06: No Target Cluster for Restore [OUTAGE]
- **Scenario**: Disaster requires restoring backups, but no active GKE cluster exists in the DR region.
- **Impact**: Cluster creation takes 20+ minutes or fails due to regional API errors, violating RTO.
- **Handling**: DR GKE clusters must be pre-provisioned via Cluster API baseline.
- **Detection**: Dependency probe `GKE_DR_CLUSTER` runs during periodic readiness checks.

### CFP-07: DR Region Capacity Exhausted (Stockouts) [OUTAGE]
- **Scenario**: Mass regional failovers exhaust cloud compute capacity in DR zones; nodes cannot join.
- **Impact**: Workload pods remain in `Pending` state indefinitely.
- **Handling**: Pre-purchased Compute Engine capacity reservations for Tier-0/1 node pools (ADR-001).
- **Detection**: Reservation coverage verification during drills; monitoring `FailedScheduling` events.

### CFP-08: Container Images Unavailable in DR [OUTAGE]
- **Scenario**: Container registry in the primary region becomes unreachable.
- **Impact**: DR nodes cannot pull workload images on restart or scale-up.
- **Handling**: Dual-region mirrored Artifact Registry repositories or sovereign in-region mirrors.
- **Detection**: Scheduled image pull verification probe executed from DR node pools.

### CFP-09: Binary Authorization Attestations Unverifiable [OUTAGE / SECURITY]
- **Scenario**: Attestor keys or KMS signer reside solely in failed primary region.
- **Impact**: GKE admission controller blocks pod scheduling in DR region.
- **Handling**: Attestor public keys mirrored in DR; DR policy identical to primary (DR-INV-12).
- **Detection**: Test deployment drill in DR cluster with strict admission enforcement enabled.

### CFP-10: GitOps Writer Lost with Primary Region [OUTAGE]
- **Scenario**: Argo CD control plane is co-located with primary region and lost during outage.
- **Impact**: DR clusters cannot synchronize configuration updates or hotfixes.
- **Handling**: Independent Argo CD instance operating in DR region with local Git mirror access.
- **Detection**: Bi-directional GitOps reconciliation probe from DR vantage point.

### CFP-11: Secrets Missing or Stale in DR [OUTAGE / SECURITY]
- **Scenario**: Secret rotation occurred on primary but failed to replicate to DR secret store.
- **Impact**: Apps crash on startup due to invalid credentials or stale database passwords.
- **Handling**: Multi-region replicated Secret Manager / Vault; continuous secret version parity check.
- **Detection**: `SECRET_PARITY` dependency probe in `CheckReadiness`.

### CFP-12: Clients Cache Dead Primary IP [OUTAGE]
- **Scenario**: Client applications or DNS resolvers ignore TTL and cache failed primary endpoints.
- **Impact**: Traffic does not shift to DR even after DNS failover completes.
- **Handling**: Ultra-low DNS TTLs (30s) pre-configured; use global external Application Load Balancer with regional failover backends.
- **Detection**: Post-cutover per-region ingress request metrics.

### CFP-13: False Failover from Flapping Monitoring [OUTAGE / CORRUPT]
- **Scenario**: Transient single-vantage network blip triggers accidental failover.
- **Impact**: Unnecessary downtime, split-brain risk, and replication lag data loss.
- **Handling**: Multi-vantage health checking (minimum 2 of 3 regions must report down) + mandatory two-person human approval (D-DECLARE).
- **Detection**: Declaration audit logging; review of vantage point probe reports.

### CFP-14: Partial Failover [DEGRADE]
- **Scenario**: Database failover succeeds, but traffic routing or compute scaling fails.
- **Impact**: Distributed system operates across WAN with excessive latency and connection timeouts.
- **Handling**: Centralized failover workflow DAG (`DRFailoverWorkflow`) treats failover as an atomic transaction across all 6 stages.
- **Detection**: Step result status mapping in `DRFailoverOperationRecord`.

### CFP-15: Backups Inconsistent or Corrupted [LOSS]
- **Scenario**: Crash-consistent backup taken without database quiescing; recovery crashes on restart.
- **Impact**: Complete data loss; inability to recover from corruption or ransomware.
- **Handling**: Application-consistent backups via Backup for GKE pre/post hooks.
- **Detection**: Automated periodic restore drills validating data integrity (`contracts.DRRestoreOperationRecord.DataValidated`).

### CFP-16: DR Cluster Configuration Drift [OUTAGE]
- **Scenario**: Primary cluster upgraded to newer Kubernetes or policy version; DR cluster left behind.
- **Impact**: Workloads fail in DR due to deprecated APIs or missing CRDs.
- **Handling**: GitOps-driven identical declarative cluster baselines (DR-INV-06).
- **Detection**: Continuous posture scan diff; drift automatically sets DR status to `NOT_READY`.

### CFP-17: Quota or IAM Mismatch in DR Project [OUTAGE]
- **Scenario**: DR project lacks sufficient CPU quota, IP allocation, or IAM bindings.
- **Impact**: Promotion or scaling operations are rejected by cloud APIs.
- **Handling**: Automated IaC quota synchronization and project parity checks.
- **Detection**: Quota headroom probes executed during monthly DR drills.

### CFP-18: Audit Trail Broken During DR [COMPLIANCE]
- **Scenario**: Audit logs stream exclusively to a bucket located in the failed region.
- **Impact**: Complete regulatory compliance failure; audit trail lost during incident.
- **Handling**: Dual-region audit sinks (DR-INV-09); logs routed to surviving DR sink.
- **Detection**: Sink write test executed during periodic drills.

### CFP-19: DR Control Tooling Unusable [OUTAGE]
- **Scenario**: DR API depends on primary region IdP, VPN, or portal that is down.
- **Impact**: Operators cannot authenticate or trigger failover.
- **Handling**: Independent break-glass identities and DR API instances deployed in both regions.
- **Detection**: Monthly tooling smoke test (`POST /api/v1/dr/breakglass:activate`).

### CFP-20: Failback Overwrites DR Writes [LOSS]
- **Scenario**: Operator initiates failback before reverse replication has reached zero or without divergence verification.
- **Impact**: Transactions processed during DR are permanently overwritten.
- **Handling**: **ADR-009**. `ValidateFailbackPrerequisites` mandates `Divergence == 0` and `ReverseLag == 0` before failback switchover.
- **Detection**: Cryptographic reconciliation report verification prior to route restore.

---

## 2. Edge Cases (EC-D01 to EC-D12)

- **EC-D01 (Replicated Corruption)**: Restore point-in-time backup rather than promoting replica.
- **EC-D02 (Ransomware Backup Deletion)**: Immutable locked backups in isolated administrative domain.
- **EC-D03 (Primary Returns Mid-Failover)**: Forward-lock state machine ensures DR promotion completes.
- **EC-D04 (Concurrent Region & Security Incident)**: Security playbook takes precedence; rebuild on clean images.
- **EC-D05 (DR Region Degraded)**: Shed lower-tier workloads; prioritize Tier-0 capacity.
- **EC-D06 (Global LB Overload)**: Regional rate-limiting and graceful degradation sheds non-critical traffic.
- **EC-D07 (Backend Bucket Incompatibility)**: Pre-provision regional storage buckets for static assets.
- **EC-D08 (Long-Lived Connections)**: Exponential backoff with jitter on client reconnection.
- **EC-D09 (Duplicate Scheduled Jobs)**: Leader election tied directly to fencing epoch.
- **EC-D10 (Session Token Loss)**: Stateless JWT authentication with distributed revocation cache.
- **EC-D11 (Sovereignty Constraint on DR Region)**: Residency (B-1) strictly limits legal region pairs.
- **EC-D12 (Drill Affecting Production)**: Synthetic test tenants and production-safe guards enforce isolation.

