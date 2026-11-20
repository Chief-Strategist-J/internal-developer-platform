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

func TestCalculateSurgePods(t *testing.T) {
	if got := rules.CalculateSurgePods(1, contracts.StrategyCanary); got != 1 {
		t.Errorf("expected 1 surge pod for 1 replica canary, got %d", got)
	}
	if got := rules.CalculateSurgePods(4, contracts.StrategyCanary); got != 1 {
		t.Errorf("expected 1 surge pod for 4 replicas canary, got %d", got)
	}
	if got := rules.CalculateSurgePods(8, contracts.StrategyRollingUpdate); got != 2 {
		t.Errorf("expected 2 surge pods for 8 replicas rolling update, got %d", got)
	}
	if got := rules.CalculateSurgePods(4, contracts.StrategyRecreate); got != 0 {
		t.Errorf("expected 0 surge pods for recreate strategy, got %d", got)
	}
}

func TestEvaluateResourceQuotaSurgeHeadroomExceeded(t *testing.T) {
	quota := contracts.ResourceQuotaSpec{
		HardCPU:    "1000m",
		HardMemory: "2Gi",
		UsedCPU:    "0m",
		UsedMemory: "0Gi",
	}
	res := rules.EvaluateResourceQuotaSurgeHeadroom(quota, "250m", "512Mi", 4, contracts.StrategyCanary)
	if res.SufficientHeadroom {
		t.Fatalf("expected surge headroom to fail because 4+1 pods * 250m = 1250m > 1000m")
	}
	if !strings.Contains(res.ViolationMessage, "quota exceeded for surge") {
		t.Errorf("expected surge violation message, got: %s", res.ViolationMessage)
	}
}

func TestComputeWorkloadSpecHashDeterministic(t *testing.T) {
	cmd1 := contracts.WorkloadProvisionCommand{
		TenantID:     "tenant-1",
		ServiceID:    "service-1",
		Namespace:    "orders-prod",
		WorkloadName: "orders-api",
		Image:        "repo/orders:v1",
		CPURequest:   "250m",
		CPULimit:     "500m",
		Replicas:     2,
		Strategy:     contracts.StrategyCanary,
	}
	cmd2 := cmd1
	hash1 := contracts.ComputeWorkloadSpecHash(cmd1)
	hash2 := contracts.ComputeWorkloadSpecHash(cmd2)
	if hash1 == "" || hash1 != hash2 {
		t.Fatalf("expected non-empty deterministic hash, got %s and %s", hash1, hash2)
	}
	cmd2.Image = "repo/orders:v2"
	if contracts.ComputeWorkloadSpecHash(cmd2) == hash1 {
		t.Fatalf("expected hash to change when image changes")
	}
}

func TestValidateSpecApprovalProductionAndTamper(t *testing.T) {
	cmd := contracts.WorkloadProvisionCommand{
		TenantID:     "tenant-1",
		Namespace:    "orders-prod",
		WorkloadName: "orders-api",
		Image:        "repo/orders:v1",
		Replicas:     2,
	}
	if err := rules.ValidateSpecApproval(cmd); err == nil || !strings.Contains(err.Error(), "require explicit approval") {
		t.Errorf("expected prod namespace to require spec approval, got: %v", err)
	}
	cmd.ApprovedSpecHash = "invalid-hash"
	if err := rules.ValidateSpecApproval(cmd); err == nil || !strings.Contains(err.Error(), "spec edited during approval") {
		t.Errorf("expected spec tamper error, got: %v", err)
	}
	cmd.ApprovedSpecHash = contracts.ComputeWorkloadSpecHash(cmd)
	if err := rules.ValidateSpecApproval(cmd); err != nil {
		t.Errorf("expected approval to pass with matching hash, got: %v", err)
	}
}

