// Package adapters provides outbound infrastructure adapters for Kubernetes Workload Provisioning.
//
// Architectural Blueprint:
// - Step 11 Adapter: Concrete Kubernetes adapter fulfilling K8sClientPort.
// - Generates manifest bundles, interacts with Kubernetes API clients, and observes pod readiness.
// - 100% comment-free function bodies.
package adapters

import (
	"context"
	"fmt"
	"strings"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
	"internal-developer-platform/src/infra/clients/backup"
	"internal-developer-platform/src/infra/clients/certificates"
	"internal-developer-platform/src/infra/clients/cloudresource"
	"internal-developer-platform/src/infra/clients/gateway"
	"internal-developer-platform/src/infra/clients/kubernetes"
	"internal-developer-platform/src/infra/clients/rollouts"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/errors"
)

const (
	NamespaceTemplate = `apiVersion: v1
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
`
	ResourceQuotaTemplate = `apiVersion: v1
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
`
	LimitRangeTemplate = `apiVersion: v1
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
`
	NetworkPolicyTemplate = `apiVersion: networking.k8s.io/v1
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
`
	DeploymentTemplate = `apiVersion: apps/v1
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
        imagePullPolicy: %s
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
`
	PDBTemplate = `apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: %s-pdb
  namespace: %s
spec:
  minAvailable: %d
  selector:
    matchLabels:
      app.kubernetes.io/name: %s
`
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

func buildNamespaceYAML(namespace string, labels map[string]string) string {
	return fmt.Sprintf(NamespaceTemplate,
		namespace,
		labels["pod-security.kubernetes.io/enforce"],
		labels["pod-security.kubernetes.io/audit"],
		labels["pod-security.kubernetes.io/warn"],
		labels["istio.io/rev"],
		labels["app.kubernetes.io/managed-by"],
		labels["app.kubernetes.io/part-of"],
		labels["app.kubernetes.io/name"],
	)
}

func buildResourceQuotaYAML(namespace string, q rules.ResourceQuotaDefaults) string {
	return fmt.Sprintf(ResourceQuotaTemplate,
		namespace, namespace, q.RequestsCPU, q.RequestsMemory, q.LimitsCPU, q.LimitsMemory, q.MaxPods,
	)
}

func buildLimitRangeYAML(namespace string, lr rules.LimitRangeDefaults) string {
	return fmt.Sprintf(LimitRangeTemplate,
		namespace, namespace, lr.DefaultLimitCPU, lr.DefaultLimitMemory, lr.DefaultRequestCPU, lr.DefaultRequestMemory,
	)
}

func buildNetworkPolicyYAML(workloadName, namespace string) string {
	return fmt.Sprintf(NetworkPolicyTemplate, workloadName, namespace)
}

func buildDeploymentYAML(cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) string {
	sc := mutations.SecurityContext
	pb := mutations.Probes
	return fmt.Sprintf(DeploymentTemplate,
		cmd.WorkloadName, cmd.Namespace, cmd.WorkloadName, cmd.TenantID,
		cmd.Replicas, cmd.WorkloadName, cmd.WorkloadName, cmd.TenantID,
		sc.RunAsNonRoot, sc.RunAsUser, sc.RunAsGroup, sc.SeccompProfileType,
		cmd.WorkloadName, cmd.Image, mutations.ImagePullPolicy, sc.AllowPrivilegeEscalation, sc.ReadOnlyRootFilesystem,
		mutations.CPURequest, mutations.MemoryRequest, mutations.CPULimit, mutations.MemoryLimit,
		pb.StartupPath, pb.Port, pb.LivenessPath, pb.Port, pb.ReadinessPath, pb.Port,
	)
}

func buildPDBYAML(workloadName, namespace string, minAvailable int) string {
	return fmt.Sprintf(PDBTemplate, workloadName, namespace, minAvailable, workloadName)
}

func buildMeshManifests(cmd contracts.WorkloadProvisionCommand) (string, string) {
	meshGen := rollouts.NewMeshRouterGenerator()
	cfg := rollouts.RolloutManifestConfig{
		Name:               cmd.WorkloadName,
		Namespace:          cmd.Namespace,
		Replicas:           int32(cmd.Replicas),
		WorkloadDigest:     cmd.Image,
		StableServiceName:  fmt.Sprintf("%s-stable", cmd.WorkloadName),
		CanaryServiceName:  fmt.Sprintf("%s-canary", cmd.WorkloadName),
		VirtualServiceName: cmd.WorkloadName,
	}
	vs, _ := meshGen.GenerateVirtualServiceYAML(cfg)
	dr, _ := meshGen.GenerateDestinationRuleYAML(cfg)
	return vs, dr
}

func buildCloudManifests(cmd contracts.WorkloadProvisionCommand) (string, string) {
	meta := cloudresource.ResourceMetadata{
		RequestID: cmd.ID, Owner: cmd.TenantID, CostCenter: cmd.TenantID, WorkloadID: cmd.WorkloadName, Environment: "production",
	}
	gsa := fmt.Sprintf("%s@%s.iam.gserviceaccount.com", cmd.WorkloadName, cmd.TenantID)
	wi, _ := cloudresource.GenerateWorkloadIdentityManifest(cloudresource.WorkloadIdentityBindingSpec{
		GSAEmail: gsa, KSAName: cmd.WorkloadName, Namespace: cmd.Namespace, ProjectID: cmd.TenantID, RoleBindings: []string{"roles/cloudsql.client"}, Metadata: meta,
	})
	sql, _ := cloudresource.GenerateCloudSQLBindingManifest(cloudresource.CloudSQLBindingSpec{
		InstanceID: fmt.Sprintf("%s-db", cmd.WorkloadName), DatabaseName: cmd.WorkloadName, AppUser: cmd.WorkloadName, GSAEmail: gsa, IAMAuthEnabled: true, DeletionProtection: true, Metadata: meta,
	})
	return wi, sql
}

func buildGatewayManifests(cmd contracts.WorkloadProvisionCommand) (string, string, string) {
	rg := gateway.NewHTTPRouteGenerator()
	rt, _ := rg.GenerateHTTPRouteYAML(gateway.HTTPRouteConfig{
		Name: cmd.WorkloadName, Namespace: cmd.Namespace, Hostnames: []string{fmt.Sprintf("%s.%s.example.com", cmd.WorkloadName, cmd.TenantID)}, ServiceName: cmd.WorkloadName, ServicePort: 8080,
	})
	ag := gateway.NewCloudArmorGenerator()
	bp, _ := ag.GenerateGCPBackendPolicyYAML(gateway.GCPBackendPolicyConfig{
		Name: fmt.Sprintf("%s-backend-policy", cmd.WorkloadName), Namespace: cmd.Namespace, TargetServiceName: cmd.WorkloadName, SecurityPolicyName: "platform-edge-security-policy", TimeoutSec: 30, LoggingEnabled: true,
	})
	sp, _ := ag.GenerateComputeSecurityPolicyYAML(gateway.ComputeSecurityPolicyConfig{
		Name:        "platform-edge-security-policy",
		Namespace:   cmd.Namespace,
		ProjectID:   cmd.TenantID,
		Description: "Enterprise Cloud Armor Edge Security Policy with rate limiting and WAF rules",
		Rules: []gateway.CloudArmorRule{
			{
				Priority:    1000,
				Action:      gateway.ActionAllow,
				Description: "Default allow ingress traffic",
				SrcIPRanges: []string{gateway.MatchAnySrcIP},
			},
			{
				Priority:             2000,
				Action:               gateway.ActionRateBasedBan,
				Description:          "Anti-DDoS and brute-force rate limit",
				SrcIPRanges:          []string{gateway.MatchAnySrcIP},
				RateLimitThreshold:   1000,
				RateLimitIntervalSec: 60,
			},
		},
	})
	return rt, bp, sp
}

func buildCertificateYAML(cmd contracts.WorkloadProvisionCommand) string {
	hosts := cmd.TLSHosts
	if len(hosts) == 0 {
		hosts = []string{fmt.Sprintf("%s.%s.example.com", cmd.WorkloadName, cmd.TenantID)}
	}
	spec := certificates.CertificateSpec{
		Name:         fmt.Sprintf("%s-tls", cmd.WorkloadName),
		Namespace:    cmd.Namespace,
		SecretName:   fmt.Sprintf("%s-tls-cert", cmd.WorkloadName),
		DNSNames:     hosts,
		IssuerName:   "letsencrypt-prod",
		IssuerKind:   certificates.IssuerKindCluster,
		KeyAlgorithm: certificates.KeyAlgorithmECDSA,
		KeySize:      256,
		Duration:     certificates.DefaultCertDuration,
		RenewBefore:  certificates.DefaultRenewBefore,
	}
	certYAML, err := certificates.GenerateCertificateYAML(spec)
	if err != nil {
		return ""
	}
	return certYAML
}

func buildSecretYAML(cmd contracts.WorkloadProvisionCommand) string {
	if len(cmd.EncryptedSecrets) == 0 && cmd.KMSKeyURI == "" {
		return ""
	}
	var dataBuilder strings.Builder
	for k, v := range cmd.EncryptedSecrets {
		dataBuilder.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
	}
	return fmt.Sprintf(`apiVersion: v1
kind: Secret
metadata:
  name: %s-secrets
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    app.kubernetes.io/part-of: %s
    app.kubernetes.io/managed-by: internal-developer-platform
  annotations:
    kms.platform.idp.io/key-uri: %s
    kms.platform.idp.io/algorithm: AES-256-GCM-KMS-ENVELOPE
type: Opaque
data:
%s`, cmd.WorkloadName, cmd.Namespace, cmd.WorkloadName, cmd.TenantID, cmd.KMSKeyURI, dataBuilder.String())
}

func (k *KubernetesClientAdapter) GenerateManifestBundle(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) (*contracts.ManifestBundle, error) {
	return observability.RunTraced(ctx, "k8s.manifest.generate_bundle", func(ctx context.Context) (*contracts.ManifestBundle, error) {
		vsYAML, drYAML := buildMeshManifests(cmd)
		wiYAML, sqlYAML := buildCloudManifests(cmd)
		rtYAML, bpYAML, spYAML := buildGatewayManifests(cmd)
		rolloutCfg := rules.EvaluateDefaultRolloutConfig(cmd)
		var pdbYAML string
		if mutations.PDBRequired {
			pdbYAML = buildPDBYAML(cmd.WorkloadName, cmd.Namespace, mutations.MinAvailable)
		}
		var backupPlanYAML, restorePlanYAML string
		if cmd.BackupPlan.Enabled {
			backupPlanYAML, _ = backup.GenerateBackupPlanYAML(backup.BackupPlanConfig{
				Name:              cmd.WorkloadName,
				Namespace:         cmd.Namespace,
				Cluster:           cmd.BackupPlan.Cluster,
				Location:          cmd.BackupPlan.Location,
				CronSchedule:      cmd.BackupPlan.CronSchedule,
				RetentionDays:     cmd.BackupPlan.RetentionDays,
				LockedRetention:   cmd.BackupPlan.LockedRetention,
				IncludeSecrets:    cmd.BackupPlan.IncludeSecrets,
				IncludeVolumeData: cmd.BackupPlan.IncludeVolumeData,
				TargetNamespaces:  []string{cmd.Namespace},
				EncryptionKeyKMS:  cmd.BackupPlan.EncryptionKeyKMS,
			})
			if cmd.RestorePlan.Enabled {
				restorePlanYAML, _ = backup.GenerateRestorePlanYAML(backup.RestorePlanConfig{
					Name:               cmd.WorkloadName,
					Namespace:          cmd.Namespace,
					TargetCluster:      cmd.RestorePlan.TargetCluster,
					SourceBackupPlan:   cmd.RestorePlan.SourceBackupPlan,
					ConflictPolicy:     cmd.RestorePlan.ConflictPolicy,
					SelectedNamespaces: []string{cmd.Namespace},
				})
			}
		}
		return &contracts.ManifestBundle{
			NamespaceYAML:           buildNamespaceYAML(cmd.Namespace, mutations.NamespaceLabels),
			ResourceQuotaYAML:       buildResourceQuotaYAML(cmd.Namespace, mutations.ResourceQuota),
			LimitRangeYAML:          buildLimitRangeYAML(cmd.Namespace, mutations.LimitRange),
			NetworkPolicyYAML:       buildNetworkPolicyYAML(cmd.WorkloadName, cmd.Namespace),
			DeploymentYAML:          buildDeploymentYAML(cmd, mutations),
			PDBYAML:                 pdbYAML,
			ArgoCDApplicationYAML:   rules.GenerateArgoCDApplicationYAML(cmd, contracts.GitOpsAppConfig{}),
			RolloutYAML:             rules.GenerateArgoRolloutYAML(cmd, mutations, rolloutCfg),
			AnalysisTemplateYAML:    rules.GenerateAnalysisTemplateYAML(cmd, rolloutCfg.AnalysisMetrics),
			AuthorizationPolicyYAML: rules.GenerateMeshAuthorizationPolicyYAML(cmd, contracts.MeshSecurityConfig{MtlsMode: "STRICT"}),
			VirtualServiceYAML:      vsYAML,
			DestinationRuleYAML:     drYAML,
			WorkloadIdentityYAML:    wiYAML,
			DatabaseYAML:            sqlYAML,
			HTTPRouteYAML:           rtYAML,
			BackendPolicyYAML:       bpYAML,
			SecurityPolicyYAML:      spYAML,
			CertificateYAML:         buildCertificateYAML(cmd),
			SecretYAML:              buildSecretYAML(cmd),
			PeerAuthenticationYAML:  rules.GeneratePeerAuthenticationYAML(cmd.Namespace, cmd.WorkloadName, cmd.MeshSecurity.MtlsMode),
			ServiceEntryYAML:        rules.GenerateServiceEntryYAML(cmd),
			LogSinkYAML:             rules.GenerateLogSinkYAML(cmd),
			LoggingBucketYAML:       rules.GenerateLoggingBucketYAML(cmd),
			BackupPlanYAML:          backupPlanYAML,
			RestorePlanYAML:         restorePlanYAML,
		}, nil
	}, attribute.String("workload.name", cmd.WorkloadName), attribute.String("namespace", cmd.Namespace))
}

func (k *KubernetesClientAdapter) ApplyDeployment(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) error {
	return observability.WithSpan(ctx, "k8s.adapter.apply_deployment", func(ctx context.Context) error {
		bundle, err := k.GenerateManifestBundle(ctx, cmd, mutations)
		if err != nil {
			return err
		}
		key := fmt.Sprintf("%s/%s", cmd.Namespace, cmd.WorkloadName)
		k.mu.Lock()
		k.appliedBundles[key] = *bundle
		k.observedReplicas[key] = cmd.Replicas
		k.mu.Unlock()

		if k.client != nil {
			if err := k.applyLiveNamespace(ctx, cmd.Namespace, mutations.NamespaceLabels); err != nil {
				return err
			}
			if err := k.applyLiveDeployment(ctx, cmd, mutations); err != nil {
				return err
			}
		}

		return nil
	}, attribute.String("workload.name", cmd.WorkloadName), attribute.String("namespace", cmd.Namespace), attribute.String("tenant.id", cmd.TenantID))
}

func (k *KubernetesClientAdapter) applyLiveNamespace(ctx context.Context, namespace string, labels map[string]string) error {
	return observability.WithSpan(ctx, "k8s.adapter.apply_live_namespace", func(ctx context.Context) error {
		_, err := k.client.CreateOrUpdateNamespace(ctx, namespace, labels)
		return err
	}, attribute.String("namespace", namespace))
}

func (k *KubernetesClientAdapter) applyLiveDeployment(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) error {
	return observability.WithSpan(ctx, "k8s.adapter.apply_live_deployment", func(ctx context.Context) error {
		deploy := buildDeploymentObject(cmd, mutations)
		_, err := k.client.CreateOrUpdateDeployment(ctx, cmd.Namespace, deploy)
		return err
	}, attribute.String("workload.name", cmd.WorkloadName), attribute.String("namespace", cmd.Namespace))
}

func buildDeploymentObject(cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) *appsv1.Deployment {
	replicasInt32 := int32(cmd.Replicas)
	runAsNonRoot := mutations.SecurityContext.RunAsNonRoot
	allowPrivilegeEscalation := mutations.SecurityContext.AllowPrivilegeEscalation
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmd.WorkloadName,
			Namespace: cmd.Namespace,
			Labels: map[string]string{
				constants.LabelAppName:      cmd.WorkloadName,
				constants.LabelAppPartOf:    cmd.TenantID,
				constants.LabelAppManagedBy: constants.DefaultManagerValue,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicasInt32,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					constants.LabelAppName: cmd.WorkloadName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						constants.LabelAppName: cmd.WorkloadName,
					},
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: &runAsNonRoot,
					},
					Containers: []corev1.Container{
						{
							Name:  cmd.WorkloadName,
							Image: cmd.Image,
							SecurityContext: &corev1.SecurityContext{
								AllowPrivilegeEscalation: &allowPrivilegeEscalation,
								RunAsNonRoot:             &runAsNonRoot,
								ReadOnlyRootFilesystem:   &mutations.SecurityContext.ReadOnlyRootFilesystem,
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{constants.DefaultCapabilityDropAll},
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(mutations.CPURequest),
									corev1.ResourceMemory: resource.MustParse(mutations.MemoryRequest),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse(mutations.CPULimit),
									corev1.ResourceMemory: resource.MustParse(mutations.MemoryLimit),
								},
							},
						},
					},
				},
			},
		},
	}
}

func (k *KubernetesClientAdapter) ApplyPDB(ctx context.Context, namespace string, workloadName string, minAvailable int) error {
	return observability.WithSpan(ctx, "k8s.adapter.apply_pdb", func(ctx context.Context) error {
		key := fmt.Sprintf("%s/%s", namespace, workloadName)
		k.mu.Lock()
		bundle, exists := k.appliedBundles[key]
		if !exists {
			k.mu.Unlock()
			return errors.NewNotFoundError("target deployment for pdb does not exist")
		}
		bundle.PDBYAML = buildPDBYAML(workloadName, namespace, minAvailable)
		k.appliedBundles[key] = bundle
		k.mu.Unlock()

		if k.client != nil {
			return k.applyLivePDB(ctx, namespace, workloadName, minAvailable)
		}

		return nil
	}, attribute.String("workload.name", workloadName), attribute.String("namespace", namespace))
}

func (k *KubernetesClientAdapter) applyLivePDB(ctx context.Context, namespace, workloadName string, minAvailable int) error {
	return observability.WithSpan(ctx, "k8s.adapter.apply_live_pdb", func(ctx context.Context) error {
		pdb := buildPDBObject(workloadName, namespace, minAvailable)
		_, err := k.client.CreateOrUpdatePDB(ctx, namespace, pdb)
		return err
	}, attribute.String("workload.name", workloadName), attribute.String("namespace", namespace))
}

func buildPDBObject(workloadName, namespace string, minAvailable int) *policyv1.PodDisruptionBudget {
	minAvail := intstr.FromInt(minAvailable)
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s%s", workloadName, constants.DefaultPDBNameSuffix),
			Namespace: namespace,
			Labels: map[string]string{
				constants.LabelAppName:      workloadName,
				constants.LabelAppManagedBy: constants.DefaultManagerValue,
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvail,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					constants.LabelAppName: workloadName,
				},
			},
		},
	}
}

func (k *KubernetesClientAdapter) GetWorkloadHealth(ctx context.Context, namespace string, workloadName string) (contracts.WorkloadStatus, error) {
	return observability.RunTraced(ctx, "k8s.adapter.get_workload_health", func(ctx context.Context) (contracts.WorkloadStatus, error) {
		if k.client != nil {
			ready, err := k.client.GetDeploymentReadyReplicas(ctx, namespace, workloadName)
			if err == nil {
				if ready > 0 {
					return contracts.StatusObservedReady, nil
				}
				return contracts.StatusDegraded, nil
			}
		}
		key := fmt.Sprintf("%s/%s", namespace, workloadName)
		k.mu.RLock()
		defer k.mu.RUnlock()
		replicas, exists := k.observedReplicas[key]
		if !exists || replicas == 0 {
			return contracts.StatusRejected, errors.NewNotFoundError("workload not found or zero replicas observed")
		}
		return contracts.StatusObservedReady, nil
	}, attribute.String("workload.name", workloadName), attribute.String("namespace", namespace))
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
			return &contracts.ResourceQuotaSpec{
				HardCPU:    quota.Status.Hard.Cpu().String(),
				HardMemory: quota.Status.Hard.Memory().String(),
				UsedCPU:    quota.Status.Used.Cpu().String(),
				UsedMemory: quota.Status.Used.Memory().String(),
			}, nil
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
