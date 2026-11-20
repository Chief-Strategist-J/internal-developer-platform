# Architecture Decision Records (ADRs): Autoscaling & Elastic Capacity Management

## ADR-001: Strict Omission of `spec.replicas` in GitOps Manifests During Autoscaling
- **Status**: Accepted
- **Date**: 2026-11-19
- **Platform Invariants**: `GIT-03`, `AVL-01`
- **Context**:
  In GitOps architectures orchestrated by Argo CD, declaring `spec.replicas: N` inside version-controlled `Deployment` or `Rollout` manifests creates an irreconcilable reconciliation loop with Horizontal Pod Autoscalers (HPA) and KEDA. Whenever an autoscaler alters replica count based on real-time traffic, Argo CD flags an `OutOfSync` state and reverts the pod replica count back to the Git baseline. This causes catastrophic deployment flapping, dropped requests, and cyclic scale thrashing during production surges.
- **Decision Drivers**:
  1. **Single Source of Operational Truth**: Replicas for scalable workloads must be governed dynamically by cluster autoscaling control loops, not static Git manifests.
  2. **Zero Git Drift False Positives**: Argo CD must remain green (`Synced`) while the autoscaler dynamically scales pod counts.
- **Decision Outcome**:
  1. **Manifest Sanitization**: When a workload configuration declares autoscaling (`minReplicas > 0` and `maxReplicas > minReplicas`), the Workload Provisioning Engine strictly omits the `spec.replicas` key from the generated `Deployment` or `Rollout` manifests committed to Git.
  2. **Argo CD Diff Suppression**: Every platform-generated Argo CD `Application` resource automatically configures `ignoreDifferences` for the replica path:
     ```yaml
     spec:
       ignoreDifferences:
         - group: apps
           kind: Deployment
           jsonPointers:
             - /spec/replicas
         - group: argoproj.io
           kind: Rollout
           jsonPointers:
             - /spec/replicas
     ```
  3. **Bootstrap Initializer**: Initial deployment replicas default to `minReplicas` via the HPA controller's own initial state declaration.
- **Consequences**:
  - *Positive*: Eliminates GitOps reconciliation thrashing; allows HPA and KEDA complete authority over real-time workload scale.
  - *Negative*: Manual replica scaling via `kubectl scale` will be overridden by the HPA; developers must adjust `minReplicas`/`maxReplicas` via the Workload API instead.

---

## ADR-002: Asymmetric Scaling Policies and Dampening Hysteresis
- **Status**: Accepted
- **Date**: 2026-11-19
- **Platform Invariants**: `AVL-02`, `SLO-01`
- **Context**:
  Network traffic patterns exhibit sharp spikes (flash sales, batch job completions, breaking news) and volatile temporary valleys. Symmetrical autoscaling policies cause catastrophic flapping: a workload scales down during a 30-second traffic dip, only to be overwhelmed and drop connections when traffic resumes 10 seconds later. Furthermore, pod initialization latencies (JVM warm-up, container image pulling) require scale-up to be immediate while scale-down must be conservative.
- **Decision Outcome**:
  Standardize all platform `HorizontalPodAutoscaler` resources on asymmetric scaling behaviors:
  1. **Metric Evaluation Equation**:
     $$\text{desiredReplicas} = \left\lceil \text{currentReplicas} \times \left( \frac{\text{currentMetricValue}}{\text{targetMetricValue}} \right) \right\rceil$$
  2. **Scale-Up Policy (Aggressive & Burst-Tolerant)**:
     - Stabilization Window: `0 seconds` (instant reaction to spikes).
     - Rate Limits: Up to $+100\%$ of current replicas (or $+4$ pods, whichever is greater) every 15 seconds.
     - Policy select: `Max` policy rule.
  3. **Scale-Down Policy (Dampened & Conservative)**:
     - Stabilization Window: `300 seconds` (5 minutes). Replica reduction is calculated using the highest replica demand observed across the entire 5-minute rolling window.
     - Rate Limits: Maximum $10\%$ reduction of current replicas per 60-second period.
     - Policy select: `Min` reduction rule.
  ```yaml
  apiVersion: autoscaling/v2
  kind: HorizontalPodAutoscaler
  metadata:
    name: tenant-service-hpa
  spec:
    scaleTargetRef:
      apiVersion: argoproj.io/v1alpha1
      kind: Rollout
      name: tenant-service
    minReplicas: 2
    maxReplicas: 20
    behavior:
      scaleUp:
        stabilizationWindowSeconds: 0
        policies:
          - type: Percent
            value: 100
            periodSeconds: 15
          - type: Pods
            value: 4
            periodSeconds: 15
        selectPolicy: Max
      scaleDown:
        stabilizationWindowSeconds: 300
        policies:
          - type: Percent
            value: 10
            periodSeconds: 60
        selectPolicy: Min
  ```
- **Consequences**:
  - *Positive*: Guarantees zero dropped connections during burst traffic; completely eliminates pod flapping and thrashing during transient lulls.
  - *Negative*: Workloads retain peak compute capacity for 5 minutes after traffic recedes, incurring slightly higher cloud compute costs in exchange for high availability.

---

## ADR-003: Sub-Minute Compute Node Expansion via Cloud Node Provisioners (Karpenter / CAS)
- **Status**: Accepted
- **Date**: 2026-11-19
- **Platform Invariants**: `AVL-03`, `TEN-04`
- **Context**:
  Pod autoscaling is futile if the underlying Kubernetes cluster lacks allocatable compute capacity. When 50 pods scale out simultaneously, they enter `Pending` state. Traditional Cluster Autoscaler (CAS) managing static managed instance groups (MIGs) takes 3 to 6 minutes to detect pending pods, calculate node group quotas, trigger cloud provider instance creation, join the kubelet, and pull images. This latency breaches application P99 latency SLOs during sudden spikes.
- **Decision Outcome**:
  1. **Direct Unschedulable Interception**: Platform integrates next-generation node provisioners (Karpenter or GKE Fast Node Autoscaler) triggered immediately by the `PodCondition: Unschedulable` signal.
  2. **Just-In-Time Instance Right-Sizing**: Instead of scaling homogeneous node pools, the node provisioner analyzes the exact aggregated CPU/memory requests, topology spread constraints, and node affinities of pending pods, instantly selecting the optimal cloud instance type (e.g., AMD EPYC `c6a.2xlarge` or Spot instances).
  3. **Sub-60s Provisioning Budget**: Node provisioning, kubelet registration, and daemonset injection must complete in $<60\text{ seconds}$.
  4. **Pre-Pulled Base Images**: Standard container base images are cached or pre-baked into custom node AMIs/OS images to eliminate image pull latency on newly joined nodes.
- **Consequences**:
  - *Positive*: Minimizes pod queuing time from 5 minutes down to $<60$ seconds, preventing cascading HTTP 504 gateway timeouts.
  - *Negative*: Requires advanced IAM node provisioning roles and careful Karpenter disruption budgeting to manage Spot instance interruptions.

---

## ADR-004: Decoupled Vertical Pod Autoscaler (VPA) in Recommendation-Only Mode
- **Status**: Accepted
- **Date**: 2026-11-19
- **Platform Invariants**: `TEN-04`, `AVL-01`
- **Context**:
  Developers routinely miscalculate container resource requests, either grossly over-provisioning memory (wasting thousands of dollars) or under-provisioning CPU (leading to CPU throttling and degraded latency). However, enabling automated in-place or eviction-based Vertical Pod Autoscaler (`updateMode: "Auto"`) in production terminates running pods to resize them, risking cascading outages if pods are evicted during unexpected production traffic spikes.
- **Decision Outcome**:
  1. **Mode Separation by Environment**:
     - **Production**: VPA is strictly enforced in `updateMode: "Off"` (Recommendation-Only mode).
     - **Staging / Dev**: VPA may operate in `updateMode: "Initial"` to right-size pods upon deployment creation.
  2. **Statistical Sizing Algorithm**:
     - Historical resource telemetry over a rolling 7-day window is ingested from Prometheus:
       $$\text{CPU Recommendation} = P95(\text{CPU Usage}) \times 1.15 \quad (\text{15\% headroom})$$
       $$\text{Memory Recommendation} = \max(\text{Memory Usage}) \times 1.20 \quad (\text{20\% OOM headroom})$$
  3. **Advisory Pipeline**:
     - VPA recommendations are converted into Prometheus gauge metrics (`idp_vpa_recommended_cpu_cores`, `idp_vpa_recommended_memory_bytes`).
     - Weekly automated right-sizing reports and Pull Requests are delivered to tenant service repositories.
     - Live pod evictions during production traffic are strictly prohibited.
- **Consequences**:
  - *Positive*: Identifies massive infrastructure cost savings without risking spontaneous pod evictions or unexpected restarts in production.
  - *Negative*: Developers must manually accept and merge right-sizing PRs rather than having resources automatically resized in place.

---

## ADR-005: Multi-Tiered Quota Ceiling Clamping and Runaway Autoscaler Defense
- **Status**: Accepted
- **Date**: 2026-11-19
- **Platform Invariants**: `TEN-01`, `TEN-04`, `SLO-02`
- **Context**:
  Software defects (e.g., an unhandled memory leak, an infinite recursive loop, or an upstream retry storm) can trigger continuous autoscaling demand. Without rigorous hard caps, an autoscaler will expand pods until it exhausts entire cluster node capacity, starves co-located tenant workloads, and incurs unbounded cloud provider billing overruns.
- **Decision Outcome**:
  1. **Immutable Maximum Ceiling**: Every autoscaling configuration must declare an explicit, immutable `maxReplicas` ceiling.
  2. **Admission Headroom Validation**:
     The Workload Admission Engine rejects any workload configuration where:
     $$\text{maxReplicas} \times \text{podResourceRequests.cpu} > \text{namespace.ResourceQuota.hard.cpu}$$
     $$\text{maxReplicas} \times \text{podResourceRequests.memory} > \text{namespace.ResourceQuota.hard.memory}$$
  3. **Ceiling Clamping & Alerting**:
     - If metric evaluation demands replicas exceeding `maxReplicas`, the autoscaling controller clamps the target count to `maxReplicas`.
     - If an autoscaler runs at its maximum ceiling for $>120\text{ seconds}$, a high-priority alert (`AutoscalerCeilingSaturated`) is dispatched to tenant on-call engineers via Alertmanager.
  4. **Downstream Circuit Breaking**:
     - In event-driven autoscaling (KEDA queue-based scaling), max replica counts are hard-clamped to prevent downstream databases (e.g., PostgreSQL connection pools) from being overwhelmed by hundreds of concurrent worker pods.
- **Consequences**:
  - *Positive*: Guarantees tenant isolation; completely prevents financial runaway scenarios and protects shared platform infrastructure from rogue workloads.
  - *Negative*: Workloads under legitimate extraordinary traffic that exceeds their configured ceiling will experience latency or queue build-up until operators approve a quota increase.
