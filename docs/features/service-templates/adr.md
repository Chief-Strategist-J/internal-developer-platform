# Architecture Decision Records (ADRs): Service Templates & Golden Path Scaffolding

## ADR-001: Scaffolding Engine as a Non-Provisioning Generator vs Autonomous Provisioner
- **Status**: Accepted
- **Date**: 2026-11-19
- **Platform Invariants**: `T-1`, `T-2`, `GIT-01`
- **Context**: 
  Standard off-the-shelf developer portals (e.g., Backstage Software Templates, Yeoman, Cookiecutter) frequently integrate actions that directly invoke cloud APIs (Google Cloud Resource Manager, AWS IAM, Terraform Cloud) or create Kubernetes clusters. In our platform architecture, allowing a scaffolding engine to autonomously provision cloud resources creates an unaudited shadow control plane. It bypasses our central in-process CEL admission engine, violates the single-writer GitOps principle where only Argo CD writes to clusters, and dramatically widens the attack surface (CVE-2024-52595 and related portal template injection vulnerabilities).
- **Decision Drivers**:
  1. **Single Entry Point**: All infrastructure mutations must flow through the central `Workload API` (`POST /api/v1/workloads`).
  2. **Auditability**: Zero out-of-band cloud resource synthesis; every resource allocation must be tracked against a validated tenant quota and cryptographic identity.
  3. **Least Privilege**: The scaffolder service must never possess cloud project creator or cluster admin permissions.
- **Considered Options**:
  - *Option 1: Scaffolder provisions repo, cloud resources, and Argo CD Application directly.* Rejected: Creates dual control plane, bypasses admission validation, and requires massive credential provisioning in the portal runner.
  - *Option 2: Scaffolder only generates Git repository with Helm/Kustomize files; developer manually runs kubectl.* Rejected: Violates golden path self-service; leads to configuration drift and human errors.
  - *Option 3: Scaffolder generates source code repository and submits a declarative Workload API request with an idempotency key (SELECTED).*
- **Decision Outcome**:
  Adopt **Option 3**:
  1. The template scaffolder renders application source code, Dockerfile, build pipeline, and platform metadata into a newly created repository on the allowlisted Git host (`D-SCM`).
  2. The scaffolder synthesizes a strongly-typed `contracts.WorkloadSpec` payload and submits it to `POST /api/v1/workloads` using a deterministic `Idempotency-Key` (UUIDv5 derived from `tenant_id + "/" + service_name`).
  3. The Workload API executes admission validation, records the workload in PostgreSQL, commits sanitized Kubernetes manifests into the tenant GitOps repository, and registers the application in Argo CD.
- **Consequences**:
  - *Positive*: Preserves strict separation of concerns; enforces uniform admission governance across all workloads; eliminates privilege escalation risks.
  - *Negative*: Requires template authors to conform to the platform's declarative Workload API schema rather than authoring arbitrary raw Helm charts.

---

## ADR-002: Sandboxed Scaffolder Runtime with Ephemeral Execution and Zero Cloud Credentials
- **Status**: Accepted
- **Date**: 2026-11-19
- **Platform Invariants**: `T-3`, `SEC-01`, `POD-04..11`
- **Context**:
  Template scaffolding engines evaluate user-provided string inputs against templating engines (Jinja2, Liquid, Mustache, Go templates). If malicious or malformed parameters are submitted, template injection can lead to Remote Code Execution (RCE) inside the scaffolder container. If the scaffolder pod carries cloud IAM credentials or Kubernetes ServiceAccount tokens with write permissions, an attacker gains cluster-level or cloud-tenant administrative control.
- **Decision Drivers**:
  - **Blast Radius Containment**: A compromised template rendering pod must not grant lateral movement into cloud infrastructure or tenant namespaces.
  - **Zero Trust**: Scaffolding execution must have no direct network access outside the designated Git host.
- **Decision Outcome**:
  1. **Container Hardening**: The scaffolder pod runs under the Kubernetes Restricted Pod Security Standard (`runAsNonRoot: true`, `runAsUser: 10001`, `allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, `capabilities.drop: ["ALL"]`, `seccompProfile: { type: "RuntimeDefault" }`).
  2. **Ephemeral Workspace**: Rendering occurs exclusively in ephemeral `emptyDir` memory mounts (`medium: Memory`, `sizeLimit: 256Mi`). Rendering workspaces are securely wiped immediately upon completion or process termination.
  3. **Zero Cloud Tokens**: The pod is assigned zero GCP Workload Identity / AWS IAM roles and mounts no Kubernetes API ServiceAccount token (`automountServiceAccountToken: false`).
  4. **Strict Egress Clamping**: A dedicated `NetworkPolicy` prohibits all outbound egress except:
     - CoreDNS (`kube-system/kube-dns:53`).
     - Allowed Git host API/SSH endpoints (e.g., GitHub Enterprise or GitLab) over TLS 443.
     - Internal IDP Workload API gateway over internal mutual TLS.
- **Consequences**:
  - *Positive*: Complete containment of template injection attacks; zero risk of stolen cloud service account tokens.
  - *Negative*: Templates cannot fetch dynamic external assets during rendering; all dependencies, schemas, and archetypes must be vendored into the template package.

---

## ADR-003: Golden Path Rule Catalog Compliance by Construction
- **Status**: Accepted
- **Date**: 2026-11-19
- **Platform Invariants**: `T-2`, `POD-04..11`, `IMG-01..04`, `AVL-01..02`, `NET-01..03`
- **Context**:
  When platform teams establish security policies after the fact, application teams face friction, broken deployment pipelines, and endless exemption requests. Shifting security left into golden path templates guarantees that any service initialized from the platform catalog satisfies 100% of the governance rules out of the box.
- **Decision Outcome**:
  Every official template family (Go, Rust, Python, Java, Node.js) must generate application artifacts that satisfy the Policy-as-Code catalog by construction:
  1. **Build & Image Governance**:
     - Standardized multi-stage Dockerfiles utilizing distroless or minimal hardened base images pinned by immutable SHA256 digest (`IMG-02/03`).
     - Automated generation of Software Bill of Materials (SBOM via Syft) and cryptographic image signatures (Cosign) integrated into the generated CI pipeline (`IMG-01`).
     - Mandatory explicit `imagePullPolicy: IfNotPresent` (`IMG-04`).
  2. **Workload Runtime Boundary**:
     - Hardened non-root runtime context with dropped Linux capabilities and `readOnlyRootFilesystem: true` (`POD-04/05/06/07/11`).
     - Automated generation of `PodDisruptionBudget` declaring `minAvailable: 1` and `unhealthyPodEvictionPolicy: AlwaysAllow` for any workload with $\ge 2$ replicas (`AVL-01/02`).
     - Default-deny Kubernetes `NetworkPolicy` providing explicit ingress allow rules for port 8080 and egress allow rules for platform telemetry collectors (`NET-01`).
     - Declarative canary rollout definitions utilizing Argo Rollouts and standardized `AnalysisTemplate` (`GIT-04`).
- **Consequences**:
  - *Positive*: Developers pass admission gates on commit #1 without manual security adjustments. Compliance audits achieve deterministic pass rates.
  - *Negative*: Developers must adhere to restricted filesystem practices (e.g., using explicit memory mounts for temp files) from day one.

---

## ADR-004: Cryptographic Template Lineage, Semantic Versioning, and Status Ledger
- **Status**: Accepted
- **Date**: 2026-11-19
- **Platform Invariants**: `T-4`, `T-5`, `SEC-04`
- **Context**:
  In large microservice fleets, platform teams struggle to inventory which services were built with deprecated framework versions, vulnerable logging libraries, or outdated Kubernetes API versions. Without machine-readable provenance, tracking tech debt and coordinating fleet-wide migrations requires manual codebase grepping.
- **Decision Outcome**:
  1. **Template SemVer & Tagging**: Every template repository is versioned via strict Semantic Versioning (`vMAJOR.MINOR.PATCH`) with signed Git commit tags (`T-4`).
  2. **Lifecycle State Machine**: Templates maintain explicit registry states: `DRAFT` $\rightarrow$ `ACTIVE` $\rightarrow$ `DEPRECATED` $\rightarrow$ `YANKED`.
  3. **Immutable Lineage Manifest**: Every scaffolding generation embeds an immutable provenance file at `.platform/lineage.json`:
     ```json
     {
       "schema_version": "1.0.0",
       "template_id": "tpl-service-go-standard",
       "template_version": "v2.4.1",
       "template_commit_sha": "9f8a3b2c1d0e4f5a6b7c8d9e0f1a2b3c4d5e6f7a",
       "scaffolded_at": "2026-11-19T13:45:00Z",
       "generator_version": "1.18.0",
       "parameters_digest": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
     }
     ```
  4. **Relational Indexing**: The Workload API persists this lineage metadata in the PostgreSQL `workloads` table (`template_id`, `template_version`, `template_sha`), enabling instantaneous SQL queries across the entire enterprise estate.
- **Consequences**:
  - *Positive*: Platform engineering can identify all services requiring security patches (e.g., when a base template is `YANKED`) via a single query.
  - *Negative*: Template authors must maintain backward-compatible semantic versioning and release notes.

---

## ADR-005: 3-Way Git Merge Engine for Non-Destructive Template Upgrades
- **Status**: Accepted
- **Date**: 2026-11-19
- **Platform Invariants**: `T-6`, `GIT-02`
- **Context**:
  Template scaffolding is traditionally a "day 1" activity: once code is generated, the connection to the template is severed. When platform standards evolve (e.g., updating OpenTelemetry SDKs, upgrading Go 1.25 to 1.26, or modifying Kubernetes manifest schemas), application teams are forced to either copy-paste changes manually or overwrite their repos, losing custom business logic.
- **Decision Outcome**:
  Implement a Git-native 3-way merge upgrade engine (`T-6`):
  1. **Base Snapshot Storage**: Upon initial scaffolding, the platform stores the exact pristine template render in a dedicated platform branch (`platform/template-base`).
  2. **Upgrade Synthesis**: When upgrading a service from Template `v1.0.0` to `v1.1.0`:
     - An ephemeral worker renders the new template `v1.1.0` using the original stored parameter answers.
     - A 3-way Git merge is executed: `Ancestor (platform/template-base @ v1.0.0)` $\times$ `Our (service default branch @ HEAD)` $\times$ `Their (rendered v1.1.0)`.
  3. **Conflict Handling**:
     - Clean diffs (e.g., updated Dockerfile, new CI workflow steps, updated telemetry libraries) are merged automatically into a dedicated Pull Request (`chore(platform): upgrade template to v1.1.0`).
     - Conflicting diffs (e.g., modifications to shared route registration or custom middleware) retain Git conflict markers and assign the PR to the designated service owners for human review.
     - Automatic force-pushes or silent overwrites of application code are strictly forbidden.
- **Consequences**:
  - *Positive*: Transforms golden path templates into evergreen scaffolding that maintains synchronization with platform evolutions over years.
  - *Negative*: Requires maintaining template parameter schemas and backwards compatibility between template minor releases.
