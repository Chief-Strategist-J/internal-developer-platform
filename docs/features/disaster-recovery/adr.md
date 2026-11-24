# Architecture Decision Records (ADRs): Disaster Recovery

## ADR-001: Static Stability & Regional Trust Anchor Independence
- **Status**: Accepted
- **Context**: On June 12, 2025, a Google Cloud policy incident crashed Service Control globally, disabling cloud API authorization across all regions. Failovers that rely on mutating cloud control planes (creating clusters, re-registering KMS keys, generating DNS zones) during a major incident fail under global control plane degradation.
- **Decision**: 
  1. The DR environment operates under the principle of **static stability**: the DR side serves traffic without requiring control plane mutations in the critical window.
  2. DR GKE clusters must be pre-created via Cluster API (restore never creates clusters on the fly).
  3. Regional KMS keys, Binary Authorization attestor keys, mirrored container repositories, and dual-region audit sinks must be pre-provisioned and verified via periodic readiness probes.
- **Consequences**: Failover execution does not depend on external cloud provisioning APIs during regional or global outages. Capacity stockouts are mitigated upfront via reserved node pools rather than on-demand scale-out.

---

## ADR-002: B-1 Residency and Distinct Regional Quorum
- **Status**: Accepted
- **Context**: Government data sovereignty mandates that data at rest, backups, and encryption keys never leave approved sovereign boundaries, while requiring geographically distinct regions to avoid correlated physical disasters.
- **Decision**:
  1. `PrimaryRegion` and `DRRegion` must be strictly distinct sovereign pairs (e.g. `us-central1` and `us-east4`).
  2. Any plan creation with identical regions or unapproved locations is rejected at the API schema validation gate.
  3. All CMEK keys and BackupPlan artifacts are confined to sovereign DR-allowed locations.
- **Consequences**: Eliminates compliance violations; guarantees physical resilience across power grids and disaster zones.

---

## ADR-003: B-2 Rigid RTO/RPO Floors per Incident Tier
- **Status**: Accepted
- **Context**: Ad-hoc SLAs lead to inconsistent infrastructure choices. Tier-0 systems (payments, stateful transaction ledger) require near-instant switchovers, whereas lower tiers can trade recovery time for cost efficiency.
- **Decision**: Enforce non-negotiable threshold ceilings per tier:
  - **TIER_0**: Target RTO $\le$ 300s (5 minutes), Target RPO = 0s (zero data loss mandate; switchover mode only).
  - **TIER_1**: Target RTO $\le$ 1800s (30 minutes), Target RPO $\le$ 300s (5 minutes).
  - **TIER_2**: Target RTO $\le$ 14400s (4 hours), Target RPO $\le$ 3600s (1 hour).
  Validation rejects any plan configuration exceeding these floors.
- **Consequences**: Tier-0 systems are forced into active replication with synchronous/semi-sync guarantees, while batch workloads use restore-to-new models.

---

## ADR-004: DR-INV-01 Single Write Authority & Fencing Epoch Protection
- **Status**: Accepted
- **Context**: After a cross-region promotion, stale clients or recovered primary nodes might attempt to write to the old primary database instance, causing silent data divergence and irrecoverable split-brain corruption.
- **Decision**:
  1. Prior to promoting any replica, the old primary write path must be fenced via IAM role revocation, network cut-off, or DB read-only lock.
  2. A dedicated `dr_fencing_epochs` table monotonically increments the fencing epoch.
  3. If fencing fails or cannot be verified, database promotion is aborted.
- **Consequences**: Prevents split-brain; clients attempting writes against stale epochs are fenced out and rejected.

---

## ADR-005: DR-INV-02 No Automatic Failback for Stateful Datasets
- **Status**: Accepted
- **Context**: Standard Cloud DNS routing policies automatically fail back when health checks pass. If a damaged primary region briefly recovers, DNS might route writes back before state reconciliation, losing writes made in DR.
- **Decision**:
  1. Automated failback is strictly forbidden for all stateful paths.
  2. Once traffic routes to the DR region, routing policies are locked (`LockRoutingPolicy`).
  3. Failback requires an explicit two-person approved failback procedure with reverse replication lag verified at 0 and zero unresolved data divergence.
- **Consequences**: Prevents accidental premature return to a flapping primary region; protects all state written during the disaster window.

---

## ADR-006: D-DECLARE Two-Person Quorum Gate for Stateful Failover
- **Status**: Accepted
- **Context**: Flapping monitoring checks or partial network partitions can cause catastrophic accidental failovers that lose data due to replication lag.
- **Decision**:
  1. Disaster declaration requires two distinct authorized actors: a Declarer who posts evidence links, and a Secondary Approver who validates the incident.
  2. Stateful failovers require a two-person gate: an Initiator and an Approver.
  3. Automatic failover is allowed solely for stateless workloads.
- **Consequences**: Prevents false failovers caused by single-vantage network hiccups; enforces human governance on data loss risk.

---

## ADR-007: Pluggable Outbound Provider Adapters (D-DATA & D-SEC)
- **Status**: Accepted
- **Context**: Different platform tiers utilize heterogeneous persistence (Cloud SQL Enterprise Plus, AlloyDB, Google Cloud Spanner) and secret distribution mechanisms (Google Secret Manager, HashiCorp Vault).
- **Decision**:
  1. Domain logic coordinates strictly against abstract interfaces (`DatabaseFailoverPort`, `TrafficRoutingPort`, `ClusterFailoverPort`, `SecretReplicationPort`).
  2. Specific cloud vendor operations are encapsulated in concrete adapter files in `infrastructure/adapters/`.
- **Consequences**: Enables swapping or polyglot multi-cloud usage via configuration or curl invocation without modifying domain state machines or rules.

---

## ADR-008: DR-INV-08 Drill Evidence Bundling and Immutable Audit Continuity
- **Status**: Accepted
- **Context**: Regulatory audits (DR-INV-08, DR-INV-09) require cryptographic proof of disaster recovery drill execution, actual RTO/RPO metrics, and verification traces that survive primary region loss.
- **Decision**:
  1. Every drill run (`POST /api/v1/dr/drills`) generates an immutable `EvidenceBundleRecord` containing table verification logs, measured recovery metrics, and a SHA-256 integrity checksum.
  2. Audit bundles are served directly via `GET /api/v1/dr/evidence/{run}` and routed to dual-region survival sinks (`auditSinkRegion`).
  3. Failures in meeting tier RTO/RPO floors flip plan status to `NOT_READY` and register an audit defect.
- **Consequences**: Satisfies government and enterprise audit compliance; prevents unverified or fabricated drill pass claims.

---

## ADR-009: Automated Divergence and Reverse Replication Verification Gate for Failback
- **Status**: Accepted
- **Context**: Executing failback while writes are still catching up or before conflicting writes are resolved causes silent data overwrites (CFP-20) and irrecoverable data loss.
- **Decision**:
  1. Failback execution (`POST /api/v1/dr/failbacks`) strictly requires dual-actor approval and automated prerequisite verification via `ValidateFailbackPrerequisites`.
  2. The system checks that `Divergence == 0` and `ReverseLag == 0` before initiating database switchovers or moving traffic.
  3. Any positive divergence count or non-zero reverse lag results in an immediate `409 Conflict` abort.
- **Consequences**: Guarantees zero data loss on failback; prevents reverting to a degraded or desynchronized primary cluster.


