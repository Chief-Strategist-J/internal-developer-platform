package tests

import (
	"strings"
	"testing"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
)

func TestEvaluateWorkloadMutationsDefaults(t *testing.T) {
	cmd := contracts.WorkloadProvisionCommand{
		ID:           "test-1",
		TenantID:     "tenant-alpha",
		ServiceID:    "service-omega",
		Namespace:    "default",
		WorkloadName: "api-workload",
		Image:        "repo/img:1.0.0",
		Replicas:     3,
	}

	mutations := rules.EvaluateWorkloadMutations(cmd)

	if !mutations.SecurityContext.RunAsNonRoot || mutations.SecurityContext.RunAsUser != 10001 {
		t.Errorf("expected non-root UID 10001 security context")
	}
	if !mutations.SecurityContext.ReadOnlyRootFilesystem {
		t.Errorf("expected read-only root filesystem")
	}
	if mutations.CPURequest != "250m" || mutations.CPULimit != "1000m" {
		t.Errorf("expected default CPU 250m/1000m, got %s/%s", mutations.CPURequest, mutations.CPULimit)
	}
	if mutations.MemoryRequest != "256Mi" || mutations.MemoryLimit != "1Gi" {
		t.Errorf("expected default Memory 256Mi/1Gi, got %s/%s", mutations.MemoryRequest, mutations.MemoryLimit)
	}
	if !mutations.PDBRequired || mutations.MinAvailable != 1 {
		t.Errorf("expected PDB with minAvailable=1 for replicas=3")
	}
}

func TestEvaluateWorkloadMutationsSingleReplica(t *testing.T) {
	cmd := contracts.WorkloadProvisionCommand{
		ID:            "test-2",
		TenantID:      "tenant-alpha",
		ServiceID:     "service-omega",
		Namespace:     "default",
		WorkloadName:  "worker-workload",
		Image:         "repo/img:1.0.0",
		CPURequest:    "500m",
		CPULimit:      "2000m",
		MemoryRequest: "512Mi",
		MemoryLimit:   "2Gi",
		Replicas:      1,
	}

	mutations := rules.EvaluateWorkloadMutations(cmd)

	if mutations.PDBRequired {
		t.Errorf("expected PDB not required for replicas=1")
	}
	if mutations.CPURequest != "500m" || mutations.CPULimit != "2000m" {
		t.Errorf("expected custom CPU values preserved")
	}
}

func TestEvaluateResourceQuotaHeadroomSufficient(t *testing.T) {
	quota := contracts.ResourceQuotaSpec{
		HardCPU:    "10",
		HardMemory: "20Gi",
		UsedCPU:    "2",
		UsedMemory: "4Gi",
	}

	result := rules.EvaluateResourceQuotaHeadroom(quota, "500m", "1Gi", 2)
	if !result.SufficientHeadroom {
		t.Fatalf("expected sufficient headroom, got violation: %s", result.ViolationMessage)
	}
}

func TestEvaluateResourceQuotaHeadroomExceeded(t *testing.T) {
	quota := contracts.ResourceQuotaSpec{
		HardCPU:    "4",
		HardMemory: "8Gi",
		UsedCPU:    "3500m",
		UsedMemory: "7Gi",
	}

	result := rules.EvaluateResourceQuotaHeadroom(quota, "1000m", "2Gi", 2)
	if result.SufficientHeadroom {
		t.Fatalf("expected insufficient headroom error")
	}
	if !strings.Contains(result.ViolationMessage, "quota exceeded") {
		t.Errorf("expected quota exceeded in violation message: %s", result.ViolationMessage)
	}
}

func TestGenerateArgoCDApplicationYAML(t *testing.T) {
	cmd := contracts.WorkloadProvisionCommand{
		TenantID:     "tenant-fintech",
		Namespace:    "fintech-prod",
		WorkloadName: "ledger-service",
	}
	cfg := contracts.GitOpsAppConfig{
		ProjectName:    "core-banking",
		RepoURL:        "https://github.com/bank/manifests.git",
		TargetRevision: "main",
	}

	yaml := rules.GenerateArgoCDApplicationYAML(cmd, cfg)
	if !strings.Contains(yaml, "kind: Application") {
		t.Errorf("expected kind: Application in YAML")
	}
	if !strings.Contains(yaml, "project: core-banking") {
		t.Errorf("expected project core-banking in YAML")
	}
	if !strings.Contains(yaml, "name: ledger-service") {
		t.Errorf("expected name ledger-service in YAML")
	}
	if !strings.Contains(yaml, "namespace: fintech-prod") {
		t.Errorf("expected destination namespace fintech-prod in YAML")
	}
}


func TestGenerateArgoRolloutYAML(t *testing.T) {
	cmd := contracts.WorkloadProvisionCommand{
		TenantID:     "tenant-commerce",
		Namespace:    "commerce-prod",
		WorkloadName: "cart-service",
		Replicas:     4,
		Image:        "gcr.io/shop/cart:v1.2.0",
	}
	mutations := rules.EvaluateWorkloadMutations(cmd)
	cfg := rules.EvaluateDefaultRolloutConfig(cmd)

	yaml := rules.GenerateArgoRolloutYAML(cmd, mutations, cfg)
	if !strings.Contains(yaml, "kind: Rollout") {
		t.Errorf("expected kind: Rollout")
	}
	if !strings.Contains(yaml, "replicas: 4") {
		t.Errorf("expected replicas: 4")
	}
	if !strings.Contains(yaml, "templateName: cart-service-canary-analysis") {
		t.Errorf("expected templateName cart-service-canary-analysis")
	}
	if !strings.Contains(yaml, "setWeight: 10") {
		t.Errorf("expected canary step setWeight: 10")
	}
}

func TestGenerateAnalysisTemplateYAML(t *testing.T) {
	cmd := contracts.WorkloadProvisionCommand{
		TenantID:     "tenant-commerce",
		Namespace:    "commerce-prod",
		WorkloadName: "cart-service",
	}
	cfg := rules.EvaluateDefaultRolloutConfig(cmd)

	yaml := rules.GenerateAnalysisTemplateYAML(cmd, cfg.AnalysisMetrics)
	if !strings.Contains(yaml, "kind: AnalysisTemplate") {
		t.Errorf("expected kind: AnalysisTemplate")
	}
	if !strings.Contains(yaml, "name: cart-service-canary-analysis") {
		t.Errorf("expected name cart-service-canary-analysis")
	}
	if !strings.Contains(yaml, "name: success-rate") {
		t.Errorf("expected success-rate metric")
	}
	if !strings.Contains(yaml, "name: latency-p99") {
		t.Errorf("expected latency-p99 metric")
	}
}

func TestGenerateMeshAuthorizationPolicyYAML(t *testing.T) {
	cmd := contracts.WorkloadProvisionCommand{
		TenantID:     "tenant-sec",
		Namespace:    "security-prod",
		WorkloadName: "vault-gateway",
	}
	secCfg := contracts.MeshSecurityConfig{
		MtlsMode: "STRICT",
		AllowedPrincipals: []string{
			"cluster.local/ns/security-prod/sa/trusted-client",
		},
		AllowedMethods: []string{"GET", "POST"},
	}

	yaml := rules.GenerateMeshAuthorizationPolicyYAML(cmd, secCfg)
	if !strings.Contains(yaml, "kind: AuthorizationPolicy") {
		t.Errorf("expected kind: AuthorizationPolicy")
	}
	if !strings.Contains(yaml, `security.istio.io/mtls: "STRICT"`) {
		t.Errorf("expected STRICT mtls label")
	}
	if !strings.Contains(yaml, "trusted-client") {
		t.Errorf("expected trusted-client principal in rules")
	}
	if !strings.Contains(yaml, "vault-gateway") {
		t.Errorf("expected workload principal in rules")
	}
}
