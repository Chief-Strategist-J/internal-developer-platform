# Container Images — Business Logic Decision Trees

## 1. Architectural Overview & Image Lifecycle Decision Tree

```
CONTAINER IMAGES ENGINE (BUILD, REGISTRY & RETENTION DECISION TREE)
│
├── 1. BUILD INGESTION & OCI PACKAGING TIER [P0]
│   └── Real-World: CI pipeline builds Docker image for Go service via Kaniko / BuildKit
│       └── Trigger: Inbound Image Build Intent
│           ├── Evaluation: Dockerfile Construction Standard
│           │   ├── Check: Is Base Image pulled from Whitelisted Internal Mirror (e.g. `cgr.dev/chainguard/static`)?
│           │   │   ├── NO ──> [REJECT: HTTP 403 Forbidden — Unapproved Public Base Image Source]
│           │   │   └── YES ──> Check: Is Build using Multi-Stage Build with Non-Root Final Stage?
│           │   │       ├── NO ──> [REJECT: Multi-stage non-root build is mandatory]
│           │   │       └── YES ──> Proceed to Container Build
│           ├── Action: Compile OCI Compliant Image
│           ├── Action: Compute Immutable Digest: `SHA256:7a8b9c...`
│           └── State: IMAGE_COMPILED
│
├── 2. REGISTRY PUSH & METADATA ATTESTATION TIER [P0]
│   └── Real-World: Publishing image to Harbor / AWS ECR with cryptographic provenance
│       └── State: PUSH_IN_PROGRESS
│           ├── Action: Push Image Blobs and Manifest List (Multi-Arch: `linux/amd64`, `linux/arm64`)
│           ├── Action: Generate Software Bill of Materials (SBOM via Syft / CycloneDX)
│           ├── Action: Cryptographically Sign Image Digest via Cosign / Sigstore Keyless OIDC
│           ├── Action: Attach SBOM and In-Toto Provenance Attestation to OCI Artifact Registry
│           └── State: IMAGE_PUBLISHED_AND_SIGNED
│
├── 3. IMAGE PROMOTION & REPLICATION TIER [P0 / P1]
│   └── Real-World: Promoting image from Dev registry to Staging and Regional Production Registries
│       └── Evaluation: Promotion Pipeline
│           ├── Check: Has Image passed Security Scanning & Integration Tests in Dev?
│           │   ├── NO ──> [BLOCK: Promotion Prohibited — Security / Test Gate Failed]
│           │   └── YES ──> Action: Execute Cross-Region Registry Replication (e.g. ECR us-east-1 -> us-west-2, eu-west-1)
│           │       ├── Check: Did all Regional Replications complete with identical SHA256 digest?
│           │       │   ├── NO ──> [HALT: Replication Corrupted / Digest Mismatch]
│           │       │   └── YES ──> State: REPLICATED_GLOBALLY
│           │
└── 4. RETENTION & CLEANUP LIFECYCLE TIER [P1]
    └── Real-World: Thousands of CI feature-branch builds accumulate 50 TB in container registry
        └── Trigger: Scheduled Garbage Collection Daemon (Weekly Cron)
            ├── Rule 1: Always Retain images currently referenced by active Kubernetes Pods in ANY cluster
            ├── Rule 2: Always Retain SemVer tagged releases (e.g. `v1.2.3`)
            ├── Rule 3: Retain untagged / PR build digests for maximum 14 Days
            └── Action: Delete unreferenced image layers -> Execute Registry Storage Garbage Collection
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Immutable Digest Pinning for Deployments
- Real-Life Incident: A deployment used mutable tag `image: app:v1`; a developer accidentally pushed broken code to tag `v1`, causing unexpected production node restarts to pull different code.
- Rationale: Image tags are mutable pointers. All production deployments must resolve and bind to an immutable 64-character SHA256 cryptographic digest (`app@sha256:...`).
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: DIGEST PINNING (P0-01)
Trigger: Workload Deployment Generation
├── Extract: Image Reference
├── Check: Does Image Reference contain `@sha256:` digest?
│   ├── YES ──> Digest Verified
│   └── NO ──> Action: Query Container Registry for exact SHA256 digest of tag
│       └── Mutate: Rewrite manifest image to `image@sha256:<immutable_digest>`
```

### Invariant P0-02: Cryptographic Cosign Signature Verification
- Real-Life Incident: An attacker breached a build server and uploaded a modified Docker image; Kubernetes pulled and executed the untrusted image.
- Rationale: Clusters must enforce that every container image is cryptographically signed by the corporate Cosign key prior to admission.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: IMAGE SIGNATURE (P0-02)
Trigger: Admission Webhook (Kyverno / Connaisseur)
├── Action: Verify Cosign Signature on Image Digest using Enterprise Root Public Key
├── Check: Signature Valid?
│   ├── YES ──> Permit Admission
│   └── NO ──> [HARD REJECT: Admission Denied — Unsigned Container Image]
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Multi-Arch Build Failure on ARM64 Node
- Real-Life Incident: An x86-only Docker image was deployed to an AWS Graviton ARM64 node, failing with cryptic `exec format error`.
- Decision Tree:
```
EDGE CASE 01: MULTI-ARCH COMPATIBILITY
Trigger: Container Image Push
├── Check: Does OCI Manifest Index contain both `linux/amd64` AND `linux/arm64`?
│   ├── YES ──> Universal Multi-Arch Image Verified
│   └── NO ──> [WARN & RESTRICT: Image tagged Single-Arch — Restrict Kubernetes scheduling to matching node architecture via nodeAffinity]
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
CONTAINER IMAGES ROADMAP
│
├── PHASE 0: DISTROLESS BASES & MULTI-ARCH PACKAGING (P0 - WEEKS 1-3)
│   ├── Standardized distroless and Chainguard non-root base image mirrors
│   ├── Multi-arch automated builds (amd64 / arm64) via BuildKit
│   └── Deliverable: Minimal footprint, secure container images (< 50 MB)
│
├── PHASE 1: COSIGN SIGNATURES & SBOM ATTESTATION (P1 - WEEKS 4-6)
│   ├── Cosign / Sigstore automated signing pipeline in CI/CD
│   ├── Syft automated SBOM generation and OCI artifact attachment
│   └── Deliverable: Cryptographically verifiable software supply chain
│
├── PHASE 2: GLOBAL REGISTRY REPLICATION & CACHING (P2 - WEEKS 7-9)
│   ├── Multi-region cross-cloud registry replication with harbor mirrors
│   ├── P2P image distribution (Kraken / Dragonfly) for sub-5s pod pulls
│   └── Deliverable: Lightning-fast container startup during 1,000-pod autoscaling
│
└── PHASE 3: CONTINUOUS GC & ANOMALY PRUNING (P3 - WEEKS 10-12)
    ├── Smart retention lifecycle daemon reclaiming cold storage
    └── Deliverable: 60% reduction in container registry cloud storage costs
```
