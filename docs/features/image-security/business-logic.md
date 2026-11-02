# Image Security — Business Logic Decision Trees

## 1. Architectural Overview & Image Security Scanning Decision Tree

```
IMAGE SECURITY ENGINE (VULNERABILITY SCANNING & ATTESTATION GATING DECISION TREE)
│
├── 1. STATIC VULNERABILITY SCANNING TIER [P0]
│   └── Real-World: CI pipeline pushes image; scanner scans OS packages and language dependencies
│       └── Trigger: Inbound Image Push Webhook (Trivy / Grype / Clair)
│           ├── Action: Extract Software Bill of Materials (SBOM) and layer manifests
│           ├── Action: Cross-reference CVE Databases (NVD, GitHub Advisory, OSV)
│           ├── Result: Generate CVE Report (List of CVE IDs, CVSS Scores, Fixed Versions)
│           └── State: SCAN_COMPLETED
│           │
├── 2. POLICY EVALUATION & SEVERITY GATING TIER [P0]
│   └── Real-World: Evaluating whether image with CRITICAL CVE can be deployed to production
│       └── Evaluation: Image Admission Policy Rules
│           ├── Rule 1: CRITICAL Severity CVEs (CVSS >= 9.0)
│           │   ├── Check: Does Image contain CRITICAL CVE with an Available Fix?
│           │   │   ├── YES ──> [HARD BLOCK: Image Admission Denied — Vulnerability Must Be Patched]
│           │   │   └── NO (No Fix Available / Zero-Day) ──> Route to Security Exception Evaluator (Tier 3)
│           ├── Rule 2: HIGH Severity CVEs (CVSS 7.0 - 8.9)
│           │   ├── Check: Are there > 0 HIGH CVEs with Fix Available older than 30 Days?
│           │   │   ├── YES ──> [BLOCK: 30-Day Remediation SLA Expired]
│           │   │   └── NO ──> Permitted with Warning
│           ├── Rule 3: Embedded Secrets & Private Keys
│           │   ├── Check: Did scanner discover embedded private keys, AWS tokens, or `.env` files?
│           │   │   ├── YES ──> [HARD BLOCK: CRITICAL Security Violation — Embedded Secret in Image]
│           │   │   └── NO ──> Secrets Scan Passed
│           └── State: POLICY_VERDICT_REACHED (PASS or HARD_BLOCK)
│           │
├── 3. SECURITY EXCEPTION & WAIVER WORKFLOW TIER [P1]
│   └── Real-World: Zero-day CVE discovered in upstream base image without available patch; release deadline critical
│       └── Evaluation: Exception Request Protocol
│           ├── Trigger: Developer submits CVE Waiver Request via Developer Portal
│           ├── Check: Does Request include Compensating Controls (e.g. WAF rule, network isolation)?
│           │   ├── NO ──> [REJECT: Exception Request Incomplete]
│           │   └── YES ──> Check: Is Exception Approved by SecOps Director?
│           │       ├── NO ──> [REJECT: Waiver Denied]
│           │       └── YES ──> Action: Issue Cryptographic Attestation Waiver (Max Duration: 14 Days)
│           │           └── State: EXCEPTION_ACTIVE (Signed Cosign Attestation attached to Image)
│           │
└── 4. RUNTIME CONTINUOUS SCANNING TIER [P0 / P1]
    └── Real-World: New zero-day vulnerability (like Log4Shell) disclosed for an image running in prod for 6 months
        └── Trigger: Daily CVE Database Sync Cron
            ├── Action: Scan all Active Running Pod Images across all clusters
            ├── Check: Is Newly Disclosed CRITICAL CVE found in Running Production Workload?
            │   ├── NO ──> Cluster Secure
            │   └── YES ──> Action: Create Sev-1 / Sev-2 Security Incident Ticket in Jira
            │       ├── Action: Alert Service Owner and SecOps Slack Channel
            │       ├── Action: Check: Is Automated Virtual Patch / Network Quarantine Rule available?
            │       │   ├── YES ──> Apply Cilium NetworkPolicy block to isolate vulnerable port
            │       │   └── NO ──> Require Emergency Patch Rollout within 24 Hours
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: Zero Critical CVEs with Available Fixes in Production
- Real-Life Incident: A service running with a known remote code execution vulnerability (Log4Shell) was compromised 3 weeks after the patch was available because no admission controller blocked it.
- Rationale: Known vulnerabilities with available upstream fixes must never pass production admission. Deployments must fail closed.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: CVE ADMISSION GATE (P0-01)
Trigger: Kubernetes Pod Admission
├── Action: Query Vulnerability Scan Cache for Image Digest
├── Check: Unfixed CRITICAL CVEs with `fixed_version != ""` present?
│   ├── YES ──> Check: Does Image possess Valid Cryptographic Security Exception Attestation?
│   │   ├── YES ──> Permit Admission with Audit Flag
│   │   └── NO ──> [HARD REJECT: Admission Webhook Rejects Pod Creation]
│   └── NO ──> Permit Admission
```

### Invariant P0-02: Cryptographic Software Bill of Materials (SBOM) Attestation
- Real-Life Incident: An open-source dependency was hijacked via supply-chain attack (event-stream); incident response took 2 weeks to identify which services contained the tainted package.
- Rationale: Every container image must have a machine-readable SBOM (CycloneDX / SPDX) cryptographically signed and queryable at runtime.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: MANDATORY SBOM (P0-02)
Trigger: Container Image Push
├── Check: Is Cryptographic SBOM Attestation attached to OCI manifest?
│   ├── NO ──> [BLOCK: Image Registry Rejects Push — Missing SBOM Attestation]
│   └── YES ──> Index Package Dependencies in Security Inventory DB
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: Scanner CVE Database Outage During Midnight Deployment
- Real-Life Incident: The external vulnerability database API had an outage; the CI/CD pipeline could not query CVEs and blocked a critical production hotfix.
- Decision Tree:
```
EDGE CASE 01: SCANNER DATABASE UNREACHABILITY
Trigger: Vulnerability Scanner returns HTTP 5xx or Connection Timeout
├── Check: Does Local Offline CVE Mirror Cache exist and is it < 24 Hours Old?
│   ├── YES ──> [FALLBACK TO LOCAL CACHE: Scan against local mirror and proceed]
│   └── NO ──> Emergency Hotfix Mode requires Dual-SecOps approval token to bypass
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
IMAGE SECURITY ROADMAP
│
├── PHASE 0: TRIVY STATIC SCANNING & ADMISSION GATING (P0 - WEEKS 1-3)
│   ├── Automated Trivy / Grype scanner in CI/CD pipeline
│   ├── Admission webhook blocking images with CRITICAL CVEs (with available fixes)
│   ├── Mandatory embedded secret detection blocking builds
│   └── Deliverable: Hardened cluster admission preventing known vulnerable containers
│
├── PHASE 1: SBOM GENERATION & SUPPLY-CHAIN PROVENANCE (P1 - WEEKS 4-6)
│   ├── Syft automated SBOM generation and Cosign attestation attachment
│   ├── Centralized software dependency inventory search across all clusters
│   └── Deliverable: Sub-60-second identification of compromised packages across fleet
│
├── PHASE 2: CONTINUOUS RUNTIME CVE MONITORING (P2 - WEEKS 7-9)
│   ├── Background scanner monitoring live running workloads against fresh CVE feeds
│   ├── Automated Jira ticket creation for newly disclosed zero-day vulnerabilities
│   └── Deliverable: Continuous day-2 vulnerability management with SLA enforcement
│
└── PHASE 3: AUTOMATED BASE IMAGE PATCHING BOT (P3 - WEEKS 10-12)
    ├── Dependabot-style automated PRs updating base images upon CVE patches
    └── Deliverable: Self-healing container supply chain with zero developer overhead
```
