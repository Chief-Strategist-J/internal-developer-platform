package tests

import (
	"strings"
	"testing"
	"time"

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
	if !strings.Contains(yaml, "group: networking.istio.io") || !strings.Contains(yaml, "/spec/http/0/route") {
		t.Errorf("expected VirtualService route ignoreDifferences to prevent sync fights")
	}
	if !strings.Contains(yaml, "group: argoproj.io") {
		t.Errorf("expected Rollout ignoreDifferences")
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
	if !strings.Contains(yaml, "inconclusiveLimit: 0") {
		t.Errorf("expected inconclusiveLimit: 0 to treat missing metric as failure")
	}
	if !strings.Contains(yaml, "consecutiveErrorLimit: 1") {
		t.Errorf("expected consecutiveErrorLimit: 1 for immediate abort on query failure")
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

func TestValidateImageSupplyChain_ApprovedRegistries(t *testing.T) {
	if !rules.IsApprovedRegistry("gcr.io/apps/pay:v1") {
		t.Errorf("expected gcr.io to be approved")
	}
	if !rules.IsApprovedRegistry("us-docker.pkg.dev/gov-prod/apps/pay:v1") {
		t.Errorf("expected pkg.dev to be approved")
	}
	if !rules.IsApprovedRegistry("ghcr.io/org/repo:v1") {
		t.Errorf("expected ghcr.io to be approved")
	}
	if !rules.IsApprovedRegistry("quay.io/org/repo:v1") {
		t.Errorf("expected quay.io to be approved")
	}
	if rules.IsApprovedRegistry("docker.io/untrusted/app:v1") {
		t.Errorf("expected docker.io to be rejected")
	}
	if rules.IsApprovedRegistry("evil.com/app:v1") {
		t.Errorf("expected evil.com to be rejected")
	}
}

func TestValidateImageSupplyChain_ProductionDigestMandate(t *testing.T) {
	cmd := contracts.WorkloadProvisionCommand{
		Namespace: "orders-prod",
		Image:     "repo/orders:v1",
	}
	ok, violations := rules.ValidateImageSupplyChain(cmd)
	if ok || len(violations) == 0 {
		t.Fatalf("expected production image without digest to fail")
	}
	if violations[0].Rule != "IMG-DIGEST-001" {
		t.Errorf("expected IMG-DIGEST-001 violation, got %s", violations[0].Rule)
	}

	cmd.Image = "repo/orders:v1@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	ok, violations = rules.ValidateImageSupplyChain(cmd)
	if !ok || len(violations) != 0 {
		t.Fatalf("expected digest-pinned image to pass in production")
	}
}

func TestValidateImageSupplyChain_BreakglassBypass(t *testing.T) {
	cmd := contracts.WorkloadProvisionCommand{
		Namespace:               "orders-prod",
		Image:                   "repo/orders:v1",
		BreakglassJustification: "SEV-1 payment gateway incident waiver",
	}
	ok, violations := rules.ValidateImageSupplyChain(cmd)
	if !ok {
		t.Fatalf("expected breakglass to allow deployment")
	}
	if len(violations) != 1 || violations[0].Rule != "IMG-BRK-001" {
		t.Errorf("expected IMG-BRK-001 breakglass warning, got %+v", violations)
	}
}

func TestValidateSecretVolumeMounts_SubPathProhibited(t *testing.T) {
	mounts := []contracts.SecretVolumeMount{
		{VolumeName: "app-secret", SecretName: "db-secret", MountPath: "/etc/secrets", SubPath: "db.key"},
	}
	ok, violations := rules.ValidateSecretVolumeMounts(mounts)
	if ok || len(violations) == 0 {
		t.Fatalf("expected subPath mount on secret to fail admission")
	}
	if violations[0].Rule != "SEC-SUBPATH-001" {
		t.Errorf("expected SEC-SUBPATH-001 violation, got %s", violations[0].Rule)
	}

	validMounts := []contracts.SecretVolumeMount{
		{VolumeName: "app-secret", SecretName: "db-secret", MountPath: "/etc/secrets"},
	}
	ok, violations = rules.ValidateSecretVolumeMounts(validMounts)
	if !ok || len(violations) != 0 {
		t.Fatalf("expected directory volume mount without subPath to pass")
	}
}

func TestValidateKMSKeyCustody(t *testing.T) {
	ok, violations := rules.ValidateKMSKeyCustody("tenant-alpha", "", map[string]string{"DB_PASSWORD": "secret"})
	if ok || len(violations) == 0 || violations[0].Rule != "SEC-KMS-001" {
		t.Fatalf("expected SEC-KMS-001 when encrypted secrets lack key URI")
	}

	sameProjectKey := "projects/tenant-alpha/locations/global/keyRings/ring/cryptoKeys/key"
	ok, violations = rules.ValidateKMSKeyCustody("tenant-alpha", sameProjectKey, nil)
	if ok || len(violations) == 0 || violations[0].Rule != "SEC-KMS-002" {
		t.Fatalf("expected SEC-KMS-002 when KMS key is in tenant project")
	}

	secProjectKey := "projects/platform-security-kms/locations/global/keyRings/ring/cryptoKeys/key"
	ok, violations = rules.ValidateKMSKeyCustody("tenant-alpha", secProjectKey, nil)
	if !ok || len(violations) != 0 {
		t.Fatalf("expected separate KMS project key to pass custody check")
	}
}

func TestEvaluateCertificateExpiry(t *testing.T) {
	renewBefore := 30 * 24 * time.Hour
	expired := rules.EvaluateCertificateExpiry(time.Now().Add(-1*time.Hour), renewBefore)
	if !expired.NeedsRenewal || expired.AlertSeverity != "CRITICAL_EXPIRED" {
		t.Errorf("expected CRITICAL_EXPIRED for expired certificate, got %+v", expired)
	}

	soon := rules.EvaluateCertificateExpiry(time.Now().Add(10*24*time.Hour), renewBefore)
	if !soon.NeedsRenewal || soon.AlertSeverity != "WARNING_EXPIRING_SOON" {
		t.Errorf("expected WARNING_EXPIRING_SOON for 10 days remaining, got %+v", soon)
	}

	healthy := rules.EvaluateCertificateExpiry(time.Now().Add(60*24*time.Hour), renewBefore)
	if healthy.NeedsRenewal || healthy.AlertSeverity != "HEALTHY" {
		t.Errorf("expected HEALTHY for 60 days remaining, got %+v", healthy)
	}
}

func TestValidateMeshSecurity_ProductionSTRICTMandate(t *testing.T) {
	ok, violations := rules.ValidateMeshSecurity("payments-prod", contracts.MeshSecurityConfig{MtlsMode: "PERMISSIVE"})
	if ok || len(violations) == 0 || violations[0].Rule != "MESH-MTLS-001" {
		t.Fatalf("expected MESH-MTLS-001 violation when prod namespace is PERMISSIVE")
	}

	ok, violations = rules.ValidateMeshSecurity("payments-prod", contracts.MeshSecurityConfig{MtlsMode: "STRICT"})
	if !ok || len(violations) != 0 {
		t.Fatalf("expected STRICT mTLS to pass in production")
	}

	ok, violations = rules.ValidateMeshSecurity("payments-dev", contracts.MeshSecurityConfig{MtlsMode: "PERMISSIVE"})
	if !ok || len(violations) != 0 {
		t.Fatalf("expected PERMISSIVE mTLS to pass in dev namespace")
	}
}

func TestValidateMeshCRDCompatibility_RejectsInClusterIstio(t *testing.T) {
	ok, violations := rules.ValidateMeshCRDCompatibility([]string{"EnvoyFilter"})
	if ok || len(violations) == 0 || violations[0].Rule != "MESH-OSS-001" {
		t.Fatalf("expected MESH-OSS-001 for EnvoyFilter")
	}

	ok, violations = rules.ValidateMeshCRDCompatibility([]string{"WasmPlugin", "istiod"})
	if ok || len(violations) == 0 || violations[0].Rule != "MESH-OSS-001" {
		t.Fatalf("expected MESH-OSS-001 for WasmPlugin/istiod")
	}

	ok, violations = rules.ValidateMeshCRDCompatibility([]string{"VirtualService", "DestinationRule"})
	if !ok || len(violations) != 0 {
		t.Fatalf("expected supported mesh resources to pass")
	}
}

func TestGeneratePeerAuthenticationAndServiceEntry(t *testing.T) {
	pa := rules.GeneratePeerAuthenticationYAML("payments-prod", "payment-service", "STRICT")
	if !strings.Contains(pa, "kind: PeerAuthentication") || !strings.Contains(pa, "mode: STRICT") {
		t.Errorf("expected PeerAuthentication with STRICT mode, got: %s", pa)
	}

	cmd := contracts.WorkloadProvisionCommand{
		WorkloadName: "payment-service",
		Namespace:    "payments-prod",
		EgressRules: []contracts.EgressRule{
			{Host: "api.stripe.com", Port: 443, Protocol: "HTTPS"},
		},
	}
	se := rules.GenerateServiceEntryYAML(cmd)
	if !strings.Contains(se, "kind: ServiceEntry") || !strings.Contains(se, "api.stripe.com") {
		t.Errorf("expected ServiceEntry with api.stripe.com, got: %s", se)
	}
}

func TestValidateAuditLogging_ProductionRequirements(t *testing.T) {
	cfg := contracts.AuditLoggingConfig{
		EnableDataAccessLogs: false,
		DestinationBucket:    "_Default",
		RetentionDays:        30,
		LockedRetention:      false,
	}
	ok, violations := rules.ValidateAuditLogging("payments-prod", cfg)
	if ok || len(violations) == 0 {
		t.Fatalf("expected audit logging violations in production")
	}

	validCfg := contracts.AuditLoggingConfig{
		EnableDataAccessLogs: true,
		DestinationBucket:    "gov-audit-logs-bucket",
		RetentionDays:        400,
		LockedRetention:      true,
		OffNodeLogCollection: true,
		PIIScrubbingEnabled:  true,
	}
	ok, violations = rules.ValidateAuditLogging("payments-prod", validCfg)
	if !ok || len(violations) != 0 {
		t.Fatalf("expected valid audit logging config to pass in production")
	}
}

func TestGenerateLogSinkAndBucketYAML(t *testing.T) {
	cmd := contracts.WorkloadProvisionCommand{
		WorkloadName: "audit-workload",
		Namespace:    "audit-prod",
		KMSKeyURI:    "projects/sec/locations/global/keyRings/ring/cryptoKeys/key",
		AuditLogging: contracts.AuditLoggingConfig{
			DestinationBucket: "gov-audit-bucket",
			RetentionDays:     400,
			LockedRetention:   true,
		},
	}
	sinkYAML := rules.GenerateLogSinkYAML(cmd)
	if !strings.Contains(sinkYAML, "kind: LoggingLogSink") || !strings.Contains(sinkYAML, "gov-audit-bucket") {
		t.Errorf("expected LoggingLogSink manifest, got: %s", sinkYAML)
	}

	bucketYAML := rules.GenerateLoggingBucketYAML(cmd)
	if !strings.Contains(bucketYAML, "kind: StorageBucket") || !strings.Contains(bucketYAML, "isLocked: true") {
		t.Errorf("expected StorageBucket manifest with isLocked: true, got: %s", bucketYAML)
	}
}

func TestValidateEphemeralDebugContainer_DigestAndRegistry(t *testing.T) {
	approved := []string{"gcr.io/corp-prod/", "europe-west3-docker.pkg.dev/gov-platform/internal/"}

	err := rules.ValidateEphemeralDebugContainer("gcr.io/corp-prod/debugger@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234", approved, true)
	if err != nil {
		t.Fatalf("expected valid debug container with digest to pass, got: %v", err)
	}

	err = rules.ValidateEphemeralDebugContainer("gcr.io/corp-prod/debugger:v1.0.0", approved, true)
	if err == nil {
		t.Fatalf("expected V-26 failure for tag reference in production, got nil")
	}

	err = rules.ValidateEphemeralDebugContainer("docker.io/library/busybox@sha256:abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234", approved, true)
	if err == nil {
		t.Fatalf("expected V-26 failure for unapproved registry, got nil")
	}
}

func TestGenerateJITRoleBindingYAML(t *testing.T) {
	yaml := rules.GenerateJITRoleBindingYAML("g-123", "ns-payments", "usr-alice", "DeveloperDebug", "PlatformAdmin", "incident triage", "2026-11-20T22:00:00Z")
	if !strings.Contains(yaml, "kind: RoleBinding") || !strings.Contains(yaml, "jit-grant-g-123") || !strings.Contains(yaml, "ns-payments") {
		t.Fatalf("generated RoleBinding YAML missing expected fields: %s", yaml)
	}
}







func TestValidateIncidentAndOnCallRules(t *testing.T) {
	t.Run("valid tier-2 with no pager and no runbook", func(t *testing.T) {
		err := rules.ValidateIncidentAndOnCallRules(contracts.IncidentTier2, "", "")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("valid tier-0 with pager and valid runbook", func(t *testing.T) {
		err := rules.ValidateIncidentAndOnCallRules(contracts.IncidentTier0, "pagerduty:platform-team", "https://runbooks.internal/platform-api")
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("tier-0 missing pager fails", func(t *testing.T) {
		err := rules.ValidateIncidentAndOnCallRules(contracts.IncidentTier0, "", "https://runbooks.internal/platform-api")
		if err == nil {
			t.Fatal("expected error for tier-0 without pager, got nil")
		}
	})

	t.Run("no runbook no page violation", func(t *testing.T) {
		err := rules.ValidateIncidentAndOnCallRules(contracts.IncidentTier2, "opsgenie:payments", "")
		if err == nil {
			t.Fatal("expected error for pager without runbook, got nil")
		}
	})

	t.Run("invalid runbook url protocol", func(t *testing.T) {
		err := rules.ValidateIncidentAndOnCallRules(contracts.IncidentTier1, "pagerduty:core", "ftp://runbooks.internal")
		if err == nil {
			t.Fatal("expected error for invalid runbook url, got nil")
		}
	})

	t.Run("backup plan valid configuration", func(t *testing.T) {
		cmd := contracts.WorkloadProvisionCommand{
			Namespace:    "payments-prod",
			WorkloadName: "payment-api",
			BackupPlan: contracts.BackupPlanConfig{
				Enabled:           true,
				Cluster:           "projects/gov-idp-prod/locations/us-central1/clusters/gke-prod",
				Location:          "us-central1",
				RetentionDays:     30,
				LockedRetention:   true,
				IncludeSecrets:    true,
				IncludeVolumeData: true,
			},
		}
		if err := rules.ValidateBackupPlanRules(cmd); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("backup plan retention below 30 days fails", func(t *testing.T) {
		cmd := contracts.WorkloadProvisionCommand{
			Namespace:    "payments-prod",
			WorkloadName: "payment-api",
			BackupPlan: contracts.BackupPlanConfig{
				Enabled:           true,
				Cluster:           "projects/gov-idp-prod/locations/us-central1/clusters/gke-prod",
				Location:          "us-central1",
				RetentionDays:     15,
				LockedRetention:   true,
				IncludeSecrets:    true,
				IncludeVolumeData: true,
			},
		}
		if err := rules.ValidateBackupPlanRules(cmd); err == nil {
			t.Fatal("expected error for retention < 30 days, got nil")
		}
	})

	t.Run("backup plan unlocked retention fails", func(t *testing.T) {
		cmd := contracts.WorkloadProvisionCommand{
			Namespace:    "payments-prod",
			WorkloadName: "payment-api",
			BackupPlan: contracts.BackupPlanConfig{
				Enabled:           true,
				Cluster:           "projects/gov-idp-prod/locations/us-central1/clusters/gke-prod",
				Location:          "us-central1",
				RetentionDays:     30,
				LockedRetention:   false,
				IncludeSecrets:    true,
				IncludeVolumeData: true,
			},
		}
		if err := rules.ValidateBackupPlanRules(cmd); err == nil {
			t.Fatal("expected error for unlocked retention, got nil")
		}
	})

	t.Run("backup plan missing secrets or volume data fails", func(t *testing.T) {
		cmd := contracts.WorkloadProvisionCommand{
			Namespace:    "payments-prod",
			WorkloadName: "payment-api",
			BackupPlan: contracts.BackupPlanConfig{
				Enabled:           true,
				Cluster:           "projects/gov-idp-prod/locations/us-central1/clusters/gke-prod",
				Location:          "us-central1",
				RetentionDays:     30,
				LockedRetention:   true,
				IncludeSecrets:    false,
				IncludeVolumeData: true,
			},
		}
		if err := rules.ValidateBackupPlanRules(cmd); err == nil {
			t.Fatal("expected error for missing secrets, got nil")
		}
	})
}
