# Architecture & Operational Diagrams: GitOps Reconciliation

This document details the runtime architecture of the GitOps publisher, deterministic Merkle tree hashing, signed commit generation, and the Argo CD single-writer reconciliation loop.

---

## 1. Deterministic Merkle Tree & GitOps Publisher Runtime Execution ASCII Tree

The following ASCII tree outlines the exact cryptographic process used to publish rendered Kubernetes and Config Connector manifests to the GitOps repository:

```text
[Manifest Publication Request] : DeterministicTreePublisher.Publish(ctx, spec)
  │
  ├── 1. Distributed Tracing & Span Setup
  │     ├── Open OTel Child Span: "gitops.manifest.publish"
  │     ├── Set Attributes: repository, branch, author.email
  │     └── Defer span.End()
  │
  ├── 2. Lexicographical Sorting & Leaf Hashing
  │     ├── Sort file paths deterministically in ascending ASCII order:
  │     │     ├── Path 1: apps/{tenant}/{app}/01-namespace.yaml
  │     │     ├── Path 2: apps/{tenant}/{app}/02-kcc-iam.yaml
  │     │     ├── Path 3: apps/{tenant}/{app}/03-deployment.yaml
  │     │     └── Path 4: apps/{tenant}/{app}/04-pdb.yaml
  │     │
  │     └── For each file:
  │           ├── Canonicalize content (trim trailing whitespace, ensure trailing newline)
  │           ├── Calculate Leaf SHA-256: LeafHash = SHA256(Path + "\x00" + Content)
  │           └── Store in ordered leaf array
  │
  ├── 3. Merkle Tree Root Calculation
  │     ├── Pairwise hash leaves upwards until a single 32-byte Root Hash is computed
  │     ├── MerkleRoot = SHA256(NodeLeft + NodeRight)
  │     └── Stamp MerkleRoot onto commit metadata: "X-GitOps-Merkle-Root: <hex>"
  │
  ├── 4. Signed Git Tree Object Generation
  │     ├── Create in-memory Git blob objects for each file
  │     ├── Build Git tree object referencing blobs with deterministic mode 100644
  │     └── Generate signed Git commit object referencing parent commit SHA
  │
  ├── 5. Resilient Git Push Execution
  │     ├── Acquire Bulkhead Slot from Resilience Executor
  │     ├── Attempt fast-forward push: git push origin {branch}
  │     │
  │     ├── ↳ If Fast-Forward Rejected (Concurrent Commit Race):
  │     │     ├── Fetch latest remote branch: git fetch origin {branch}
  │     │     ├── Rebase local commit cleanly onto new remote HEAD
  │     │     ├── Recalculate commit tree with updated parent pointer
  │     │     └── Retry push using exponential backoff with full jitter
  │     │
  │     └── ⚠ On Persistent Failure / Retry Budget Exhaustion:
  │           ├── RecordError(span, pushErr)
  │           └── Return 503 DownstreamUnavailableError
  │
  └── 6. Trigger Argo CD Synchronous Sync
        ├── Invoke ArgoCDClient.SyncApplication(ctx, appName, commitSHA)
        └── Poll ArgoCDClient.GetApplicationHealth(ctx, appName) until 'Healthy'
```

---

## 2. Argo CD Single-Writer Closed Loop Sequence Diagram

```mermaid
sequenceDiagram
    autonumber
    participant WorkloadSVC as Workload Service
    participant Publisher as GitOps Committer
    participant GitRepo as GitOps Repository (GitHub)
    participant ArgoCD as Argo CD Application Controller
    participant K8s as Target Kubernetes Cluster

    WorkloadSVC->>Publisher: PublishManifests(ctx, manifests)
    Publisher->>Publisher: Compute SHA-256 Merkle Tree
    Publisher->>GitRepo: git push origin main (Signed Commit)
    GitRepo-->>Publisher: Commit Accepted (SHA: 7a8b9c)
    Publisher-->>WorkloadSVC: Publication Success

    WorkloadSVC->>ArgoCD: TriggerAppSync(appName, targetSHA)
    ArgoCD->>GitRepo: git fetch origin main
    GitRepo-->>ArgoCD: Return Tree (SHA: 7a8b9c)

    ArgoCD->>K8s: Compare Live State vs Desired Tree
    alt Cluster State Has Drifted / Missing Resources
        ArgoCD->>K8s: Apply Declarative Manifests (kubectl replace/apply)
        K8s-->>ArgoCD: Resources Created (Deployment, Service, KCC)
    else Live State Matches Exactly
        ArgoCD-->>ArgoCD: Mark State Synchronized
    end

    loop Poll Health Status (Interval: 1s, Timeout: 30s)
        WorkloadSVC->>ArgoCD: GetApplicationHealth(appName)
        ArgoCD->>K8s: Check Pod Replicas & Endpoints
        K8s-->>ArgoCD: Pods Ready (3/3 Available)
        ArgoCD-->>WorkloadSVC: Status: HEALTHY
    end

    WorkloadSVC->>WorkloadSVC: Transition Workload to ACTIVE
```

---

## 3. Automated Drift Detection & Self-Healing Architecture

```mermaid
flowchart TD
    DESIRED[("Desired State<br/>(GitOps Git Repository)")]
    ARGO["Argo CD Application Controller"]
    LIVE[("Live Cluster State<br/>(GKE Cluster)")]

    DESIRED -- "1. Continuous Polling<br/>(Webhook or 3m Sync)" --> ARGO
    LIVE -- "2. Informer Cache Watch" --> ARGO

    ARGO --> DIFF{"Diff Detected?"}
    DIFF -- "No (Synced)" --> HEALTH{"Health Status?"}
    
    HEALTH -- "Healthy" --> GREEN["Status: HEALTHY (Green)"]
    HEALTH -- "Progressing" --> YELLOW["Status: PROGRESSING"]
    HEALTH -- "Degraded" --> RED["Status: DEGRADED → Alert SRE"]

    DIFF -- "Yes (Out of Sync)" --> POLICY{"Automated Self-Heal<br/>Enabled?"}
    
    POLICY -- "True (Production Standard)" --> HEAL["Execute Synchronous Prune & Apply<br/>(Overwrite Manual Console Edits)"]
    POLICY -- "False (Breakglass Mode)" --> ALERT["Fire Drift Alert (Loki/PagerDuty)"]
    
    HEAL --> LIVE
```
