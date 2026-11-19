package adapters

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
	"internal-developer-platform/src/infra/clients/cloudresource"
	"internal-developer-platform/src/infra/clients/kubernetes"
	"internal-developer-platform/src/infra/clients/rollouts"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type KubernetesClientAdapter struct {
	clusterEndpoint  string
	client           *kubernetes.KubernetesClient
	mu               sync.RWMutex
	appliedBundles   map[string]contracts.ManifestBundle
	observedReplicas map[string]int
	quotas           map[string]contracts.ResourceQuotaSpec
}

func NewKubernetesClientAdapter(endpoint string) *KubernetesClientAdapter {
	return &KubernetesClientAdapter{
		clusterEndpoint:  endpoint,
		client:           nil,
		appliedBundles:   make(map[string]contracts.ManifestBundle),
		observedReplicas: make(map[string]int),
		quotas: map[string]contracts.ResourceQuotaSpec{
			"default": {
				HardCPU:    "10",
				HardMemory: "20Gi",
				UsedCPU:    "2",
				UsedMemory: "4Gi",
			},
		},
	}
}

func NewLiveKubernetesClientAdapter(client *kubernetes.KubernetesClient) *KubernetesClientAdapter {
	adapter := NewKubernetesClientAdapter("in-cluster")
	adapter.client = client
	return adapter
}

func (k *KubernetesClientAdapter) GenerateManifestBundle(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) (*contracts.ManifestBundle, error) {
	tracer := observability.GetTracer()
	_, span := tracer.StartSpan(ctx, "k8s.manifest.generate_bundle",
		attribute.String("workload.name", cmd.WorkloadName),
		attribute.String("namespace", cmd.Namespace),
	)
	defer span.End()

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

	argoYAML := rules.GenerateArgoCDApplicationYAML(cmd, contracts.GitOpsAppConfig{})
	rolloutCfg := rules.EvaluateDefaultRolloutConfig(cmd)
	rolloutYAML := rules.GenerateArgoRolloutYAML(cmd, mutations, rolloutCfg)
	analysisYAML := rules.GenerateAnalysisTemplateYAML(cmd, rolloutCfg.AnalysisMetrics)
	authzYAML := rules.GenerateMeshAuthorizationPolicyYAML(cmd, contracts.MeshSecurityConfig{MtlsMode: "STRICT"})

	meshGen := rollouts.NewMeshRouterGenerator()
	rolloutManifestCfg := rollouts.RolloutManifestConfig{
		Name:               cmd.WorkloadName,
		Namespace:          cmd.Namespace,
		Replicas:           int32(cmd.Replicas),
		WorkloadDigest:     cmd.Image,
		StableServiceName:  fmt.Sprintf("%s-stable", cmd.WorkloadName),
		CanaryServiceName:  fmt.Sprintf("%s-canary", cmd.WorkloadName),
		VirtualServiceName: cmd.WorkloadName,
	}
	vsYAML, _ := meshGen.GenerateVirtualServiceYAML(rolloutManifestCfg)
	drYAML, _ := meshGen.GenerateDestinationRuleYAML(rolloutManifestCfg)

	gsaEmail := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", cmd.WorkloadName, cmd.TenantID)
	meta := cloudresource.ResourceMetadata{
		RequestID:   cmd.ID,
		Owner:       cmd.TenantID,
		CostCenter:  cmd.TenantID,
		WorkloadID:  cmd.WorkloadName,
		Environment: "production",
	}
	wiSpec := cloudresource.WorkloadIdentityBindingSpec{
		GSAEmail:     gsaEmail,
		KSAName:      cmd.WorkloadName,
		Namespace:    cmd.Namespace,
		ProjectID:    cmd.TenantID,
		RoleBindings: []string{"roles/cloudsql.client"},
		Metadata:     meta,
	}
	wiYAML, _ := cloudresource.GenerateWorkloadIdentityManifest(wiSpec)

	sqlSpec := cloudresource.CloudSQLBindingSpec{
		InstanceID:         fmt.Sprintf("%s-db", cmd.WorkloadName),
		DatabaseName:       cmd.WorkloadName,
		AppUser:            cmd.WorkloadName,
		GSAEmail:           gsaEmail,
		IAMAuthEnabled:     true,
		DeletionProtection: true,
		Metadata:           meta,
	}
	sqlYAML, _ := cloudresource.GenerateCloudSQLBindingManifest(sqlSpec)

	return &contracts.ManifestBundle{
		NamespaceYAML:           nsYAML,
		ResourceQuotaYAML:       quotaYAML,
		LimitRangeYAML:          limitRangeYAML,
		NetworkPolicyYAML:       netpolYAML,
		DeploymentYAML:          deployYAML,
		PDBYAML:                 pdbYAML,
		ArgoCDApplicationYAML:   argoYAML,
		RolloutYAML:             rolloutYAML,
		AnalysisTemplateYAML:    analysisYAML,
		AuthorizationPolicyYAML: authzYAML,
		VirtualServiceYAML:      vsYAML,
		DestinationRuleYAML:     drYAML,
		WorkloadIdentityYAML:    wiYAML,
		DatabaseYAML:            sqlYAML,
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

	return contracts.StatusObservedReady, nil
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

func (k *KubernetesClientAdapter) GetNamespaceQuota(ctx context.Context, namespace string) (*contracts.ResourceQuotaSpec, error) {
	if k.client != nil {
		quota, err := k.client.GetResourceQuota(ctx, namespace, "compute-quota")
		if err == nil && quota != nil {
			spec := contracts.ResourceQuotaSpec{
				HardCPU:    quota.Status.Hard.Cpu().String(),
				HardMemory: quota.Status.Hard.Memory().String(),
				UsedCPU:    quota.Status.Used.Cpu().String(),
				UsedMemory: quota.Status.Used.Memory().String(),
			}
			return &spec, nil
		}
	}

	k.mu.RLock()
	defer k.mu.RUnlock()

	if q, exists := k.quotas[namespace]; exists {
		return &q, nil
	}
	defaultQuota := k.quotas["default"]
	return &defaultQuota, nil
}

func (k *KubernetesClientAdapter) SetNamespaceQuota(namespace string, quota contracts.ResourceQuotaSpec) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.quotas[namespace] = quota
}

func (k *KubernetesClientAdapter) ObserveWorkloadReadiness(ctx context.Context, namespace string, workloadName string, desiredReplicas int) (bool, error) {
	if k.client != nil {
		ready, err := k.client.GetDeploymentReadyReplicas(ctx, namespace, workloadName)
		if err == nil {
			return int(ready) >= desiredReplicas, nil
		}
	}

	key := fmt.Sprintf("%s/%s", namespace, workloadName)
	k.mu.RLock()
	defer k.mu.RUnlock()

	observed, exists := k.observedReplicas[key]
	if !exists {
		return false, nil
	}
	return observed >= desiredReplicas, nil
}
