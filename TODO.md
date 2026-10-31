# TODO — Internal Developer Platform (Senior/Staff System Architect prep)

Topics: Kubernetes, GitOps, Terraform, ArgoCD, SRE, platform engineering.
Primary company fit: **Google** (SRE culture, Borg/Kubernetes heritage, GCP), **Anthropic** (GPU/accelerator fleets, safe deploys, reliability, security), **Stripe** (deploy safety, developer productivity, infra reliability), Neo4j (Aura managed-service operations, multi-cloud operators).

Current state: scaffold + 25 feature `context.yml`; only `src/features/service-catalog/index.go` has code. `deploy/{k8s,argocd,terraform,helm,topology}` exist — verify each is real, renders, and plans.

Every WP has an **AC**. Not done until demonstrated.

---

## 0. Definition of done for "interview-ready"
- [ ] Can design a multi-region, multi-tenant Kubernetes platform for 500+ services and defend isolation boundaries
- [ ] Can explain every control loop involved in a deploy from `git push` to traffic shift
- [ ] Can run a mock incident (detect -> mitigate -> root cause -> postmortem) and narrate it
- [ ] Working local environment (kind/k3d + ArgoCD + Terraform with localstack or a free-tier cloud) demonstrating the golden path end-to-end

## 1. Architecture artifacts (`docs/`)
- [ ] `00-platform-product.md`: personas (service dev, SRE, security, finance), golden paths, platform SLAs, adoption KPIs (onboarding time, % services on paved road, DORA four keys, developer satisfaction)
- [ ] `01-c4.md` + golden-path sequence diagram: commit -> CI -> image build/sign -> manifest PR -> ArgoCD sync -> progressive rollout -> SLO check -> promote/rollback
- [ ] `02-topology.md`: accounts/projects/clusters per env; regions; control-plane vs workload clusters; blast-radius table (what a single failure takes down)
- [ ] `03-tenancy.md`: namespace-per-team, ResourceQuota, LimitRange, NetworkPolicy default-deny, Pod Security Standards, RBAC, node pool isolation, hierarchical namespaces; hard vs soft multi-tenancy
- [ ] `04-network.md`: CIDR plan, CNI choice (Cilium/Calico), ingress/Gateway API, service mesh decision, egress control, DNS, private clusters
- [ ] `05-security.md`: supply chain (SLSA levels, SBOM, cosign, admission verification), workload identity (no static creds), secrets flow, CIS benchmark, runtime detection (Falco), threat model
- [ ] `06-capacity-and-cost.md`: bin-packing, requests vs limits policy, overprovisioning for burst, spot/on-demand mix, Karpenter, commitment strategy, cost per service
- [ ] `07-dr.md`: RTO/RPO per tier; etcd/Velero backups; cluster rebuild from code time; regional failover; **restore drill results**
- [ ] `08-slo.md`: platform's own SLOs (deploy success rate, provisioning time, control-plane availability), error budget policy
- [ ] `09-runbooks/`: failed sync, node NotReady, DNS failure, cert expiry, quota exhaustion, bad rollout, leaked secret, control-plane upgrade
- [ ] ADRs:
  - [ ] ADR-001 Build vs buy: Backstage / Port / Humanitec / custom
  - [ ] ADR-002 GitOps pull (ArgoCD/Flux) vs push CI; repo layout (mono vs poly; env-per-folder vs env-per-branch)
  - [ ] ADR-003 Cluster topology: few big clusters vs many small; cluster-per-env vs shared
  - [ ] ADR-004 Terraform vs Crossplane vs Pulumi; state layout, module versioning, drift detection
  - [ ] ADR-005 Secrets: ESO + Vault/KMS vs SOPS vs sealed-secrets
  - [ ] ADR-006 Policy: Kyverno vs OPA Gatekeeper; audit -> enforce rollout
  - [ ] ADR-007 Service mesh: none / Linkerd / Istio ambient / Cilium
  - [ ] ADR-008 Progressive delivery: Argo Rollouts vs Flagger; metric providers
  - [ ] ADR-009 Observability stack: Prometheus+Thanos/Mimir, Loki, Tempo, OpenTelemetry; cost and cardinality controls
  - [ ] ADR-010 Autoscaling: HPA/VPA/KEDA + Cluster Autoscaler vs Karpenter

## 2. Work packages

### WP-1 Service catalog + portal (`service-catalog`, `developer-portal`)
- [ ] Extend `index.go`: entity model (service, owner team, tier, lifecycle, dependencies, runbook, on-call, SLOs, repo)
- [ ] Ownership is mandatory — a service without owner can't deploy
- [ ] **AC:** deploy gate checks catalog entry; dependency graph exported (feeds `knowledge-graph-platform` as a real dataset)

### WP-2 Templates + self-service (`service-templates`, `self-service-provisioning`)
- [ ] Template creates repo, CI pipeline, Dockerfile, Helm chart/Kustomize, dashboards, alerts, SLO, catalog entry
- [ ] Idempotent request handling (same request id -> same result); reversible (deprovision + cleanup)
- [ ] **AC:** new service live with traffic, dashboards and alerts in < 15 minutes with zero tickets (measure and record)

### WP-3 Infrastructure as code (`infrastructure-as-code`)
- [ ] Module library with semantic versions; remote state with locking; per-env state split; no `terraform apply` from laptops
- [ ] Plan in PR, apply on merge, policy checks (Checkov/OPA/conftest), cost estimate (Infracost), drift detection job
- [ ] Tests: `terraform validate`, `terraform test`, Terratest for key modules
- [ ] **AC:** deliberately introduce drift and a destructive change; pipeline detects/blocks

### WP-4 GitOps (`gitops-reconciliation`)
- [ ] App-of-apps/ApplicationSet; sync waves & hooks; health checks; automated prune + selfHeal policy per env; ignoreDifferences for HPA-managed replicas
- [ ] Promotion: image tag bump PR via automation, env-by-env; rollback = git revert
- [ ] **AC:** kill ArgoCD for 1 h — workloads keep running; on restore it reconciles without surprise; document behavior

### WP-5 Progressive delivery (`deployment-strategies`, `automated-rollback`)
- [ ] Rolling (maxSurge/maxUnavailable), blue/green, canary with weighted traffic and analysis (error rate, p99, saturation, business metric)
- [ ] Auto-abort on SLO burn; manual promotion gate for tier-0 services
- [ ] **AC:** a bad build is auto-rolled back within N minutes with < X% users impacted (record numbers)

### WP-6 Workloads + environments (`kubernetes-workload-provisioning`, `environment-management`, `health-checks`)
- [ ] Defaults injected: requests/limits, probes (startup/readiness/liveness semantics — liveness must not depend on downstreams), PDB, topology spread, securityContext (non-root, read-only FS, drop caps), seccomp
- [ ] Ephemeral preview environments with TTL and cost cap
- [ ] **AC:** policy engine rejects a workload missing any default; rejection message tells the developer how to fix

### WP-7 Autoscaling (`autoscaling`)
- [ ] HPA on CPU + custom/queue metric (KEDA), VPA in recommend mode, Cluster Autoscaler/Karpenter with consolidation, PDB-aware
- [ ] **AC:** load test shows scale-out latency end-to-end (pending pod -> node -> ready) and scale-in without dropping requests (preStop + terminationGracePeriod)

### WP-8 Security (`secrets-management`, `image-security`, `policy-as-code`, `role-based-access-control`, `identity-integration`)
- [ ] OIDC SSO -> group-to-role mapping; short-lived credentials; break-glass with approval + audit + auto-expiry
- [ ] Workload identity (GKE WI / IRSA) — no long-lived keys
- [ ] Image pipeline: build -> scan -> SBOM -> sign (cosign) -> admission verify; base-image update automation
- [ ] Secrets rotated automatically; app reloads without restart or with rolling restart
- [ ] Policies in audit mode first, then enforce with exemptions process
- [ ] **AC:** leaked-secret game day: rotate, find blast radius from audit logs in < 1 h

### WP-9 Observability + SLOs + incidents (`centralized-logs`, `centralized-metrics`, `distributed-tracing`, `slo-management`, `incident-management`)
- [ ] OpenTelemetry collector pipeline; sampling strategy (head vs tail); metric cardinality budgets; log retention tiers
- [ ] SLO as code (OpenSLO/Sloth); multi-window multi-burn-rate alerts (e.g. 2% budget/1h and 5%/6h); alert on symptoms not causes
- [ ] Incident process: severity matrix, IC role, comms template, timeline, blameless postmortem template with action-item tracking
- [ ] **AC:** run one tabletop + one live game day; produce a real postmortem document

### WP-10 Cost + DR (`cost-allocation`, `disaster-recovery`, `platform-audit-trail`)
- [ ] Mandatory labels (team, service, env, cost-center) enforced by policy; showback dashboard; idle/over-provisioned report
- [ ] Backups: etcd (managed or self), Velero for PV + resources, DB snapshots; restore tested quarterly
- [ ] **AC:** rebuild a cluster from scratch from repo + backups; record actual RTO

### WP-11 Platform plumbing
- [ ] Fill `database/migrations`, `messaging/schemas`; verify Helm lint/template, kubeconform, ArgoCD app diffs, Terraform plan
- [ ] Decide `rust-modules`/`cpp-modules` purpose (e.g. fast manifest linter) or delete

## 3. Deep-dive concepts to master
- [ ] Kubernetes control plane: kube-apiserver (authn -> authz -> admission mutating -> validation -> validating -> etcd), watch/informer/work-queue pattern, controller reconcile loop, level-triggered vs edge-triggered
- [ ] etcd: Raft, MVCC, compaction, defrag, size limits (8 GB), why it is the scaling bottleneck
- [ ] Scheduler: filter/score, affinity, taints/tolerations, topology spread, preemption, priority classes, descheduler
- [ ] Kubelet, CRI, CNI (iptables vs eBPF), kube-proxy modes, CoreDNS and `ndots:5` latency trap, CSI, storage classes, StatefulSet semantics
- [ ] Requests vs limits; QoS classes; CPU throttling (CFS quota), OOM kill, memory vs cgroup v2
- [ ] Borg paper -> Kubernetes lineage; Omega; "Borg, Omega, and Kubernetes" (ACM Queue)
- [ ] Operators/CRDs: when to write one, finalizers, owner refs, status conditions
- [ ] Multi-cluster: Fleet, Cluster API, Karmada; service discovery across clusters
- [ ] Supply chain: SLSA, in-toto, Sigstore, Binary Authorization
- [ ] Cell-based architecture and blast-radius reduction; shuffle sharding
- [ ] Deployment safety: staged rollout by region, bake time, feature flags vs deploys, dark launches
- [ ] SRE: error budgets, toil, cascading failure, retry amplification, load shedding, graceful degradation, overload handling (Google SRE chapters)
- [ ] GPU/accelerator scheduling basics: device plugins, gang scheduling (Kueue/Volcano), topology-aware placement, bin-packing expensive nodes, preemption of training vs serving (Anthropic relevance)

## 4. Failure drills (automate where possible; document otherwise)
- [ ] D1 destructive `terraform apply` caught by plan review/`prevent_destroy`
- [ ] D2 ArgoCD sync loop / repo outage
- [ ] D3 node pool exhaustion; PDB blocks drain
- [ ] D4 control-plane upgrade with removed API versions (pluto/kubent scan)
- [ ] D5 leaked secret rotation
- [ ] D6 DNS failure (CoreDNS overload, `ndots`)
- [ ] D7 cert expiry (cert-manager down)
- [ ] D8 region outage -> failover; measure RTO/RPO
- [ ] D9 canary passes metrics but breaks rare path (add synthetic checks)
- [ ] D10 OOM-kill crash loop cascade
- [ ] D11 etcd disk full / slow disk
- [ ] D12 noisy neighbour exhausts node; quota/limits prevent it
- [ ] D13 bad admission webhook blocks all pod creation (failurePolicy, namespace exclusions!)
- **AC:** each has a written hypothesis, repro, detection signal, mitigation, and runbook

## 5. Experiments
- [ ] Time budget of golden path: commit -> serving traffic (break down each stage)
- [ ] Pod startup latency breakdown (image pull, init, probes)
- [ ] Scale-out latency at 10/100/1000 pods
- [ ] Cost comparison: on-demand vs spot vs Karpenter consolidation on the same load
- [ ] Control plane load: API server QPS/latency with N controllers
- **AC:** numbers recorded in `docs/10-benchmarks.md` with conclusions

## 6. Company-specific mapping

### Google
- [ ] Be fluent in GCP: GKE (Autopilot vs Standard), Cloud Run, VPC/Shared VPC/VPC-SC, IAM and org policy, Cloud Build/Deploy, Artifact Registry, Cloud Monitoring (use `llm-obs-policies/gcd` notes)
- [ ] SRE-style design: SLO-first, canarying, cell architecture, automated rollback — practice "design a global deployment system" (Google-style: Rollouts across regions with bake time)
- [ ] Understand Borg concepts: cells, allocs, priority/preemption, quota; talk about Kubernetes as its descendant
- [ ] "Googleyness": prepare stories on collaboration, humility, handling ambiguity, ethical decisions

### Anthropic
- [ ] Platform for accelerators: scheduling GPU/TPU/Trainium jobs, gang scheduling, checkpointing, preemption policy between research and serving, capacity planning across clouds
- [ ] Safety-minded infra: strong isolation for sandboxes executing model-generated code (gVisor/Firecracker/Kata, network egress deny-by-default, seccomp, per-job identity), audit trails, least privilege
- [ ] Secure-by-default deploy pipeline; insider-risk controls; model weight protection (access tiers, encryption, break-glass)
- [ ] Multi-cloud reliability: cloud outage playbooks, dependency mapping, graceful degradation of inference
- [ ] Read: Anthropic engineering posts, Responsible Scaling Policy (security levels), Claude Code sandboxing posts

### Stripe
- [ ] Deploy safety for money-moving services: staged rollouts, automatic canary analysis, feature flags, kill switches; "boring technology" philosophy
- [ ] Developer productivity: measure and shorten feedback loops; paved roads with escape hatches

### Neo4j
- [ ] Managed-database operations (Aura): K8s operators for stateful clusters, rolling upgrades of Raft clusters, backup/restore, multi-tenant isolation, noisy neighbour on disk/page cache, cost per customer
- [ ] StatefulSet specifics: ordered updates, PVC retention, anti-affinity, PDB for quorum safety (never evict more than 1 core member)

## 7. Mock interview bank
1. Design a global CI/CD + deployment system for 5000 services. *Strong: staged regions, bake, health-based gates, rollback, config vs code, blast radius, audit.*
2. Design a multi-tenant Kubernetes platform. *Strong: isolation layers, quotas, policy, identity, cost, upgrade strategy.*
3. A deploy at 14:00 raised error rate — walk through it. *Strong: mitigate first (rollback), then investigate; comms; timeline.*
4. How does `kubectl apply` travel through the cluster? *Strong: authn/z, admission, etcd, controllers, scheduler, kubelet, CNI.*
5. Design a feature-flag service. *Strong: evaluation locally with SDK cache, streaming updates, consistency, audit, kill switch.*
6. Design a secrets management system. *Strong: envelope encryption, KMS, rotation, leasing, audit, break-glass.*
7. How do you upgrade 200 clusters safely? *Strong: canary clusters, API deprecation scan, surge nodes, rollback limits, automation, fleet dashboards.*
8. Design a GPU job scheduler for training + serving. *Strong: queues, quotas, gang scheduling, preemption, checkpointing, topology, fairness.*
9. Design a sandbox to run untrusted code from an LLM. *Strong: microVM/gVisor, no creds, egress control, resource limits, ephemeral FS, audit, timeouts.*
10. How do you reduce cloud cost by 30% without hurting reliability? *Strong: measure, right-size, spot for tolerant, autoscaling, storage tiering, commitments, chargeback.*
11. Terraform state corrupted/lost — recovery? *Strong: versioned backend, import, state surgery, prevention.*
12. How do you convince 50 teams to adopt the platform? *Strong: product mindset, pain-first, paved road, metrics, no mandates without value.*

## 8. Reading list
1. Google SRE book + SRE workbook (SLOs, cascading failures, overload, postmortems, release engineering)
2. "Borg, Omega, and Kubernetes"; Borg paper
3. Kubernetes in Action / Kubernetes Patterns; official docs on scheduler, API concepts
4. Team Topologies; Platform Engineering whitepaper (CNCF); Accelerate
5. Terraform: Up & Running; ArgoCD docs; Argo Rollouts docs
6. Cilium/eBPF docs overview; SLSA spec
7. AWS/Google "Builders' Library"-style articles: static stability, avoiding fallback, workload isolation using shuffle sharding

## 9. Weekly cadence
- 1 WP + AC; 1 ADR; 1 drill/benchmark; 1 timed mock (alternate infra/SRE and general design); 1 reading; 1 behavioral story
