/*
Package adapters provides infrastructure adapters for the Kubernetes client.
The KubernetesClientAdapter generates RFC-compliant, CIS-hardened Kubernetes manifests
including Namespaces, ResourceQuotas, LimitRanges, NetworkPolicies, Deployments, and
PodDisruptionBudgets. It simulates cluster application, tracks observed replica states,
and ensures that a workload is marked READY only when available replicas are observed.
*/
package adapters

import (
	"context"
	"fmt"
	"sync"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
	"internal-developer-platform/src/shared/errors"
)

type KubernetesClientAdapter struct {
	clusterEndpoint  string
	mu               sync.RWMutex
	appliedBundles   map[string]contracts.ManifestBundle
	observedReplicas map[string]int
}

func NewKubernetesClientAdapter(endpoint string) *KubernetesClientAdapter {
	return &KubernetesClientAdapter{
		clusterEndpoint:  endpoint,
		appliedBundles:   make(map[string]contracts.ManifestBundle),
		observedReplicas: make(map[string]int),
	}
}

func (k *KubernetesClientAdapter) GenerateManifestBundle(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) (*contracts.ManifestBundle, error) {
	nsYAML := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    pod-security.kubernetes.io/enforce: %s
    pod-security.kubernetes.io/audit: %s
    pod-security.kubernetes.io/warn: %s
    istio.io/rev: %s
    app.kubernetes.io/managed-by: %s
    app.kubernetes.io/part-of: %s
    app.kubernetes.io/name: %s
`,
		cmd.Namespace,
		mutations.NamespaceLabels["pod-security.kubernetes.io/enforce"],
		mutations.NamespaceLabels["pod-security.kubernetes.io/audit"],
		mutations.NamespaceLabels["pod-security.kubernetes.io/warn"],
		mutations.NamespaceLabels["istio.io/rev"],
		mutations.NamespaceLabels["app.kubernetes.io/managed-by"],
		mutations.NamespaceLabels["app.kubernetes.io/part-of"],
		mutations.NamespaceLabels["app.kubernetes.io/name"],
	)

	quotaYAML := fmt.Sprintf(`apiVersion: v1
kind: ResourceQuota
metadata:
  name: %s-quota
  namespace: %s
spec:
  hard:
    requests.cpu: "%s"
    requests.memory: "%s"
    limits.cpu: "%s"
    limits.memory: "%s"
    pods: "%s"
`,
		cmd.Namespace,
		cmd.Namespace,
		mutations.ResourceQuota.RequestsCPU,
		mutations.ResourceQuota.RequestsMemory,
		mutations.ResourceQuota.LimitsCPU,
		mutations.ResourceQuota.LimitsMemory,
		mutations.ResourceQuota.MaxPods,
	)

	limitRangeYAML := fmt.Sprintf(`apiVersion: v1
kind: LimitRange
metadata:
  name: %s-limits
  namespace: %s
spec:
  limits:
  - default:
      cpu: "%s"
      memory: "%s"
    defaultRequest:
      cpu: "%s"
      memory: "%s"
    type: Container
`,
		cmd.Namespace,
		cmd.Namespace,
		mutations.LimitRange.DefaultLimitCPU,
		mutations.LimitRange.DefaultLimitMemory,
		mutations.LimitRange.DefaultRequestCPU,
		mutations.LimitRange.DefaultRequestMemory,
	)

	netpolYAML := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: %s-default-deny
  namespace: %s
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
  egress:
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: UDP
      port: 53
    - protocol: TCP
      port: 53
`,
		cmd.WorkloadName,
		cmd.Namespace,
	)

	deployYAML := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    app.kubernetes.io/part-of: %s
spec:
  replicas: %d
  selector:
    matchLabels:
      app.kubernetes.io/name: %s
  template:
    metadata:
      labels:
        app.kubernetes.io/name: %s
        app.kubernetes.io/part-of: %s
    spec:
      securityContext:
        runAsNonRoot: %t
        runAsUser: %d
        runAsGroup: %d
        seccompProfile:
          type: %s
      containers:
      - name: %s
        image: %s
        securityContext:
          allowPrivilegeEscalation: %t
          readOnlyRootFilesystem: %t
          capabilities:
            drop:
            - ALL
        resources:
          requests:
            cpu: "%s"
            memory: "%s"
          limits:
            cpu: "%s"
            memory: "%s"
        startupProbe:
          httpGet:
            path: %s
            port: %d
        livenessProbe:
          httpGet:
            path: %s
            port: %d
        readinessProbe:
          httpGet:
            path: %s
            port: %d
`,
		cmd.WorkloadName,
		cmd.Namespace,
		cmd.WorkloadName,
		cmd.TenantID,
		cmd.Replicas,
		cmd.WorkloadName,
		cmd.WorkloadName,
		cmd.TenantID,
		mutations.SecurityContext.RunAsNonRoot,
		mutations.SecurityContext.RunAsUser,
		mutations.SecurityContext.RunAsGroup,
		mutations.SecurityContext.SeccompProfileType,
		cmd.WorkloadName,
		cmd.Image,
		mutations.SecurityContext.AllowPrivilegeEscalation,
		mutations.SecurityContext.ReadOnlyRootFilesystem,
		mutations.CPURequest,
		mutations.MemoryRequest,
		mutations.CPULimit,
		mutations.MemoryLimit,
		mutations.Probes.StartupPath,
		mutations.Probes.Port,
		mutations.Probes.LivenessPath,
		mutations.Probes.Port,
		mutations.Probes.ReadinessPath,
		mutations.Probes.Port,
	)

	var pdbYAML string
	if mutations.PDBRequired {
		pdbYAML = fmt.Sprintf(`apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: %s-pdb
  namespace: %s
spec:
  minAvailable: %d
  selector:
    matchLabels:
      app.kubernetes.io/name: %s
`,
			cmd.WorkloadName,
			cmd.Namespace,
			mutations.MinAvailable,
			cmd.WorkloadName,
		)
	}

	return &contracts.ManifestBundle{
		NamespaceYAML:     nsYAML,
		ResourceQuotaYAML: quotaYAML,
		LimitRangeYAML:    limitRangeYAML,
		NetworkPolicyYAML: netpolYAML,
		DeploymentYAML:    deployYAML,
		PDBYAML:           pdbYAML,
	}, nil
}

func (k *KubernetesClientAdapter) ApplyDeployment(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) error {
	bundle, err := k.GenerateManifestBundle(ctx, cmd, mutations)
	if err != nil {
		return err
	}

	key := fmt.Sprintf("%s/%s", cmd.Namespace, cmd.WorkloadName)
	k.mu.Lock()
	defer k.mu.Unlock()

	k.appliedBundles[key] = *bundle
	k.observedReplicas[key] = cmd.Replicas
	return nil
}

func (k *KubernetesClientAdapter) ApplyPDB(ctx context.Context, namespace string, workloadName string, minAvailable int) error {
	key := fmt.Sprintf("%s/%s", namespace, workloadName)
	k.mu.Lock()
	defer k.mu.Unlock()

	bundle, exists := k.appliedBundles[key]
	if !exists {
		return errors.NewNotFoundError("target deployment for pdb does not exist")
	}

	pdbYAML := fmt.Sprintf(`apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: %s-pdb
  namespace: %s
spec:
  minAvailable: %d
  selector:
    matchLabels:
      app.kubernetes.io/name: %s
`,
		workloadName,
		namespace,
		minAvailable,
		workloadName,
	)

	bundle.PDBYAML = pdbYAML
	k.appliedBundles[key] = bundle
	return nil
}

func (k *KubernetesClientAdapter) GetWorkloadHealth(ctx context.Context, namespace string, workloadName string) (contracts.WorkloadStatus, error) {
	key := fmt.Sprintf("%s/%s", namespace, workloadName)
	k.mu.RLock()
	defer k.mu.RUnlock()

	replicas, exists := k.observedReplicas[key]
	if !exists || replicas == 0 {
		return contracts.StatusRejected, errors.NewNotFoundError("workload not found or zero replicas observed")
	}

	return contracts.StatusActive, nil
}

func (k *KubernetesClientAdapter) GetAppliedManifests(ctx context.Context, namespace string, workloadName string) (*contracts.ManifestBundle, error) {
	key := fmt.Sprintf("%s/%s", namespace, workloadName)
	k.mu.RLock()
	defer k.mu.RUnlock()

	bundle, exists := k.appliedBundles[key]
	if !exists {
		return nil, errors.NewNotFoundError("manifest bundle not found for workload")
	}
	return &bundle, nil
}
