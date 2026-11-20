package tests

import (
	"context"
	"strings"
	"testing"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/workflows"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/adapters"
)

func TestGenerateManifestBundleCompliant(t *testing.T) {
	ctx := context.Background()
	client := adapters.NewKubernetesClientAdapter("https://10.0.0.1:6443")

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-manifest-01",
		TenantID:       "tenant-payments",
		ServiceID:      "payment-api",
		Namespace:      "payments-prod",
		WorkloadName:   "payment-processor",
		Image:          "gcr.io/bank/payment:v2.1.0",
		CPURequest:     "500m",
		CPULimit:       "2000m",
		MemoryRequest:  "1Gi",
		MemoryLimit:    "4Gi",
		Replicas:       3,
		IdempotencyKey: "idemp-manifest-01",
	}

	mutations := rules.EvaluateWorkloadMutations(cmd)
	bundle, err := client.GenerateManifestBundle(ctx, cmd, mutations)
	if err != nil {
		t.Fatalf("unexpected error generating manifest bundle: %v", err)
	}

	if !strings.Contains(bundle.NamespaceYAML, "pod-security.kubernetes.io/enforce: restricted") {
		t.Errorf("expected restricted pod security enforce label on namespace")
	}
	if !strings.Contains(bundle.NamespaceYAML, "istio.io/rev: asm-managed") {
		t.Errorf("expected asm-managed mesh revision label on namespace")
	}

	if !strings.Contains(bundle.ResourceQuotaYAML, "kind: ResourceQuota") {
		t.Errorf("expected ResourceQuota manifest in bundle")
	}
	if !strings.Contains(bundle.LimitRangeYAML, "kind: LimitRange") {
		t.Errorf("expected LimitRange manifest in bundle")
	}
	if !strings.Contains(bundle.NetworkPolicyYAML, "kind: NetworkPolicy") {
		t.Errorf("expected NetworkPolicy manifest in bundle")
	}
	if !strings.Contains(bundle.NetworkPolicyYAML, "port: 53") {
		t.Errorf("expected DNS egress port 53 allowed in default-deny NetworkPolicy")
	}

	if !strings.Contains(bundle.DeploymentYAML, "runAsNonRoot: true") {
		t.Errorf("expected runAsNonRoot: true in deployment")
	}
	if !strings.Contains(bundle.DeploymentYAML, "runAsUser: 10001") {
		t.Errorf("expected runAsUser: 10001 in deployment")
	}
	if !strings.Contains(bundle.DeploymentYAML, "readOnlyRootFilesystem: true") {
		t.Errorf("expected readOnlyRootFilesystem: true in deployment")
	}
	if !strings.Contains(bundle.DeploymentYAML, "allowPrivilegeEscalation: false") {
		t.Errorf("expected allowPrivilegeEscalation: false in deployment")
	}
	if !strings.Contains(bundle.DeploymentYAML, "path: /readyz") {
		t.Errorf("expected readiness probe path /readyz in deployment")
	}

	if bundle.PDBYAML == "" {
		t.Errorf("expected PDB manifest for replicas=3")
	}
	if !strings.Contains(bundle.PDBYAML, "minAvailable: 1") {
		t.Errorf("expected minAvailable: 1 in PDB")
	}

	if !strings.Contains(bundle.ArgoCDApplicationYAML, "kind: Application") {
		t.Errorf("expected Argo CD Application manifest in bundle")
	}
	if !strings.Contains(bundle.ArgoCDApplicationYAML, "argoproj.io/v1alpha1") {
		t.Errorf("expected argoproj.io/v1alpha1 apiVersion in Argo CD manifest")
	}

	if !strings.Contains(bundle.RolloutYAML, "kind: Rollout") {
		t.Errorf("expected kind: Rollout in bundle")
	}
	if !strings.Contains(bundle.RolloutYAML, "setWeight: 10") {
		t.Errorf("expected canary steps in rollout")
	}
	if !strings.Contains(bundle.AnalysisTemplateYAML, "kind: AnalysisTemplate") {
		t.Errorf("expected kind: AnalysisTemplate in bundle")
	}
	if !strings.Contains(bundle.AnalysisTemplateYAML, "success-rate") {
		t.Errorf("expected success-rate metric in analysis template")
	}
	if !strings.Contains(bundle.AuthorizationPolicyYAML, "kind: AuthorizationPolicy") {
		t.Errorf("expected kind: AuthorizationPolicy in bundle")
	}
	if !strings.Contains(bundle.AuthorizationPolicyYAML, `security.istio.io/mtls: "STRICT"`) {
		t.Errorf("expected mTLS STRICT in authorization policy")
	}

	if !strings.Contains(bundle.VirtualServiceYAML, "kind: VirtualService") {
		t.Errorf("expected kind: VirtualService in bundle")
	}
	if !strings.Contains(bundle.DestinationRuleYAML, "kind: DestinationRule") {
		t.Errorf("expected kind: DestinationRule in bundle")
	}
	if !strings.Contains(bundle.WorkloadIdentityYAML, "kind: IAMPolicyMember") {
		t.Errorf("expected kind: IAMPolicyMember in bundle")
	}
	if !strings.Contains(bundle.DatabaseYAML, "kind: SQLDatabase") {
		t.Errorf("expected kind: SQLDatabase in bundle")
	}
}

func TestExecuteProvisionDAG_SynthesizesCanaryAndCloudBundle(t *testing.T) {
	ctx := context.Background()
	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-canary-dag-01",
		TenantID:       "tenant-dag",
		ServiceID:      "dag-api",
		Namespace:      "dag-prod",
		WorkloadName:   "dag-worker",
		Image:          "gcr.io/bank/dag:v1.0.0",
		CPURequest:     "500m",
		CPULimit:       "1000m",
		MemoryRequest:  "1Gi",
		MemoryLimit:    "2Gi",
		Replicas:       3,
		IdempotencyKey: "idemp-dag-01",
	}

	mutations := rules.EvaluateWorkloadMutations(cmd)
	exec, bundle, err := workflows.ExecuteProvisionDAG(ctx, cmd, mutations)
	if err != nil {
		t.Fatalf("unexpected error executing DAG: %v", err)
	}

	if len(exec.Steps) != 8 {
		t.Errorf("expected 8 DAG steps, got %d", len(exec.Steps))
	}

	if bundle == nil {
		t.Fatalf("expected non-nil synthesized canary bundle")
	}

	if !strings.Contains(bundle.RolloutYAML, "kind: Rollout") {
		t.Errorf("expected kind: Rollout in synthesized bundle")
	}
	if !strings.Contains(bundle.VirtualServiceYAML, "kind: VirtualService") {
		t.Errorf("expected kind: VirtualService in synthesized bundle")
	}
	if !strings.Contains(bundle.DestinationRuleYAML, "kind: DestinationRule") {
		t.Errorf("expected kind: DestinationRule in synthesized bundle")
	}
	if !strings.Contains(bundle.WorkloadIdentityYAML, "kind: IAMPolicyMember") {
		t.Errorf("expected kind: IAMPolicyMember in synthesized bundle")
	}
	if !strings.Contains(bundle.DatabaseYAML, "kind: SQLDatabase") {
		t.Errorf("expected kind: SQLDatabase in synthesized bundle")
	}
}

func TestAdmissionValidationRejectsRoot(t *testing.T) {
	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-root-01",
		TenantID:       "tenant-bad",
		ServiceID:      "bad-service",
		Namespace:      "default",
		WorkloadName:   "root-runner",
		Image:          "gcr.io/bank/bad:1.0.0",
		Replicas:       2,
		IdempotencyKey: "idemp-root-01",
	}

	mutations := rules.EvaluateWorkloadMutations(cmd)
	mutations.SecurityContext.RunAsNonRoot = false
	mutations.SecurityContext.RunAsUser = 0

	report, err := rules.ValidateAdmission(cmd, mutations)
	if err == nil {
		t.Fatalf("expected error validating root workload")
	}
	if report.Admitted {
		t.Errorf("expected Admitted=false for root workload")
	}
	if len(report.Violations) == 0 {
		t.Errorf("expected admission violations reported")
	}
}

func TestAdmissionValidationRejectsPrivilegeEscalation(t *testing.T) {
	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-priv-01",
		TenantID:       "tenant-bad",
		ServiceID:      "bad-service",
		Namespace:      "default",
		WorkloadName:   "escalation-runner",
		Image:          "gcr.io/bank/bad:1.0.0",
		Replicas:       2,
		IdempotencyKey: "idemp-priv-01",
	}

	mutations := rules.EvaluateWorkloadMutations(cmd)
	mutations.SecurityContext.AllowPrivilegeEscalation = true

	report, err := rules.ValidateAdmission(cmd, mutations)
	if err == nil {
		t.Fatalf("expected error validating privilege escalation workload")
	}
	if report.Admitted {
		t.Errorf("expected Admitted=false for privilege escalation workload")
	}
}

func TestKubernetesAdapterHealthCheck(t *testing.T) {
	ctx := context.Background()
	client := adapters.NewKubernetesClientAdapter("https://10.0.0.1:6443")

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-health-01",
		TenantID:       "tenant-core",
		ServiceID:      "core-api",
		Namespace:      "core-prod",
		WorkloadName:   "core-worker",
		Image:          "gcr.io/bank/core:v1.0.0",
		Replicas:       2,
		IdempotencyKey: "idemp-health-01",
	}

	mutations := rules.EvaluateWorkloadMutations(cmd)
	if err := client.ApplyDeployment(ctx, cmd, mutations); err != nil {
		t.Fatalf("unexpected error applying deployment: %v", err)
	}

	status, err := client.GetWorkloadHealth(ctx, cmd.Namespace, cmd.WorkloadName)
	if err != nil {
		t.Fatalf("unexpected error getting workload health: %v", err)
	}
	if status != contracts.StatusActive && status != contracts.StatusObservedReady {
		t.Errorf("expected status ACTIVE or OBSERVED_READY, got %s", status)
	}

	applied, err := client.GetAppliedManifests(ctx, cmd.Namespace, cmd.WorkloadName)
	if err != nil {
		t.Fatalf("unexpected error getting applied manifests: %v", err)
	}
	if applied == nil || applied.DeploymentYAML == "" {
		t.Errorf("expected non-empty deployment manifest in applied bundle")
	}
}

func TestGenerateManifestBundle_CertAndSecretManifests(t *testing.T) {
	ctx := context.Background()
	client := adapters.NewKubernetesClientAdapter("https://10.0.0.1:6443")
	cmd := contracts.WorkloadProvisionCommand{
		ID:               "wkld-manifest-sec-01",
		TenantID:         "tenant-payments",
		ServiceID:        "payment-api",
		Namespace:        "payments-prod",
		WorkloadName:     "payment-processor",
		Image:            "gcr.io/bank/payment@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		CPURequest:       "500m",
		CPULimit:         "2000m",
		MemoryRequest:    "1Gi",
		MemoryLimit:      "4Gi",
		Replicas:         2,
		IdempotencyKey:   "idemp-manifest-sec-01",
		KMSKeyURI:        "projects/platform-security-kms/locations/global/keyRings/ring/cryptoKeys/key",
		EncryptedSecrets: map[string]string{"DB_PASSWORD": "encrypted-dek-payload"},
		TLSHosts:         []string{"pay.example.gov"},
	}

	mutations := rules.EvaluateWorkloadMutations(cmd)
	bundle, err := client.GenerateManifestBundle(ctx, cmd, mutations)
	if err != nil {
		t.Fatalf("unexpected error generating manifest bundle: %v", err)
	}
	if !strings.Contains(bundle.CertificateYAML, "kind: Certificate") {
		t.Errorf("expected Certificate manifest in bundle")
	}
	if !strings.Contains(bundle.CertificateYAML, "pay.example.gov") {
		t.Errorf("expected pay.example.gov dns name in Certificate")
	}
	if !strings.Contains(bundle.SecretYAML, "kind: Secret") {
		t.Errorf("expected Secret manifest in bundle")
	}
	if !strings.Contains(bundle.SecretYAML, "AES-256-GCM-KMS-ENVELOPE") {
		t.Errorf("expected KMS envelope algorithm annotation in Secret")
	}
}

func TestGenerateManifestBundle_PeerAuthenticationAndServiceEntry(t *testing.T) {
	ctx := context.Background()
	client := adapters.NewKubernetesClientAdapter("https://10.0.0.1:6443")
	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-manifest-mesh-01",
		TenantID:       "tenant-payments",
		ServiceID:      "payment-api",
		Namespace:      "payments-prod",
		WorkloadName:   "payment-processor",
		Image:          "gcr.io/bank/payment@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Replicas:       2,
		IdempotencyKey: "idemp-manifest-mesh-01",
		MeshSecurity:   contracts.MeshSecurityConfig{MtlsMode: "STRICT"},
		EgressRules: []contracts.EgressRule{
			{Host: "api.stripe.com", Port: 443, Protocol: "HTTPS"},
		},
	}

	mutations := rules.EvaluateWorkloadMutations(cmd)
	bundle, err := client.GenerateManifestBundle(ctx, cmd, mutations)
	if err != nil {
		t.Fatalf("unexpected error generating manifest bundle: %v", err)
	}
	if !strings.Contains(bundle.PeerAuthenticationYAML, "kind: PeerAuthentication") {
		t.Errorf("expected PeerAuthentication manifest in bundle")
	}
	if !strings.Contains(bundle.PeerAuthenticationYAML, "mode: STRICT") {
		t.Errorf("expected mode: STRICT in PeerAuthentication")
	}
	if !strings.Contains(bundle.ServiceEntryYAML, "kind: ServiceEntry") {
		t.Errorf("expected ServiceEntry manifest in bundle")
	}
	if !strings.Contains(bundle.ServiceEntryYAML, "api.stripe.com") {
		t.Errorf("expected api.stripe.com host in ServiceEntry")
	}
}


