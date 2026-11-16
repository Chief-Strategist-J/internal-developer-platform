/*
Package tests verifies manifest generation, admission validation, and compliance with the
CIS GKE benchmark for the Kubernetes Workload Provisioning hexagonal feature.
*/
package tests

import (
	"context"
	"strings"
	"testing"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
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
	if status != contracts.StatusActive {
		t.Errorf("expected status ACTIVE, got %s", status)
	}

	applied, err := client.GetAppliedManifests(ctx, cmd.Namespace, cmd.WorkloadName)
	if err != nil {
		t.Fatalf("unexpected error getting applied manifests: %v", err)
	}
	if applied == nil || applied.DeploymentYAML == "" {
		t.Errorf("expected non-empty deployment manifest in applied bundle")
	}
}
