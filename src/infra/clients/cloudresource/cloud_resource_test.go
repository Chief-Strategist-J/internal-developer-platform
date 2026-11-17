package cloudresource_test

import (
	"context"
	"strings"
	"testing"

	"internal-developer-platform/src/infra/clients/cloudresource"
	"internal-developer-platform/src/infra/clients/resilience"
)

func defaultMetadata(workloadID string) cloudresource.ResourceMetadata {
	return cloudresource.ResourceMetadata{
		RequestID:   "req-12345",
		Owner:       "secops-core",
		CostCenter:  "cc-8831",
		WorkloadID:  workloadID,
		Environment: "production",
	}
}

func TestWorkloadIdentityManifestGeneration(t *testing.T) {
	client := cloudresource.NewCloudResourceClient(resilience.DefaultResilienceConfig())
	ctx := context.Background()

	spec := cloudresource.WorkloadIdentityBindingSpec{
		GSAEmail:  "payments-sa@gov-prod.iam.gserviceaccount.com",
		KSAName:   "payments-service",
		Namespace: "payments",
		ProjectID: "gov-prod",
		RoleBindings: []string{
			"roles/cloudkms.cryptoKeyEncrypterDecrypter",
			"roles/secretmanager.secretAccessor",
		},
		Metadata: defaultMetadata("wl-payments-01"),
	}

	result, err := client.ProvisionWorkloadIdentity(ctx, spec)
	if err != nil {
		t.Fatalf("unexpected error provisioning workload identity: %v", err)
	}

	if result.ResourceID != "wi-payments-payments-service" {
		t.Errorf("expected resource id wi-payments-payments-service, got %s", result.ResourceID)
	}

	yamlContent := result.ManifestYAML
	if !strings.Contains(yamlContent, "roles/iam.workloadIdentityUser") {
		t.Errorf("manifest missing workloadIdentityUser role")
	}
	if !strings.Contains(yamlContent, "serviceAccount:gov-prod.svc.id.goog[payments/payments-service]") {
		t.Errorf("manifest missing mapped workload identity member")
	}
	if !strings.Contains(yamlContent, "roles/cloudkms.cryptoKeyEncrypterDecrypter") {
		t.Errorf("manifest missing KMS role binding")
	}
	if !strings.Contains(yamlContent, "platform.gov/request-id: req-12345") {
		t.Errorf("manifest missing request-id label")
	}
	if !strings.Contains(yamlContent, "platform.gov/cost-center: cc-8831") {
		t.Errorf("manifest missing cost-center label")
	}
}

func TestWorkloadIdentityOverbroadRoleRejection(t *testing.T) {
	client := cloudresource.NewCloudResourceClient(resilience.DefaultResilienceConfig())
	ctx := context.Background()

	spec := cloudresource.WorkloadIdentityBindingSpec{
		GSAEmail:     "admin-sa@gov-prod.iam.gserviceaccount.com",
		KSAName:      "admin-service",
		Namespace:    "admin",
		ProjectID:    "gov-prod",
		RoleBindings: []string{"roles/owner"},
		Metadata:     defaultMetadata("wl-admin-01"),
	}

	_, err := client.ProvisionWorkloadIdentity(ctx, spec)
	if err == nil {
		t.Fatalf("expected error for prohibited role roles/owner, got nil")
	}
	if !strings.Contains(err.Error(), "prohibited over-broad IAM role") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDNSRecordManifestGeneration(t *testing.T) {
	client := cloudresource.NewCloudResourceClient(resilience.DefaultResilienceConfig())
	ctx := context.Background()

	spec := cloudresource.DNSRecordSpec{
		ZoneName:   "internal-gov-zone",
		RecordName: "api.payments.internal.gov",
		RecordType: cloudresource.DNSTypeA,
		TTL:        300,
		TargetIPs:  []string{"10.240.0.15", "10.240.0.16"},
		Metadata:   defaultMetadata("wl-payments-01"),
	}

	result, err := client.ProvisionDNSRecord(ctx, spec)
	if err != nil {
		t.Fatalf("unexpected error provisioning DNS record: %v", err)
	}

	if !strings.Contains(result.ManifestYAML, "api.payments.internal.gov.") {
		t.Errorf("manifest missing trailing dot FQDN in DNS record")
	}
	if !strings.Contains(result.ManifestYAML, "10.240.0.15") {
		t.Errorf("manifest missing target IP")
	}
}

func TestCloudSQLBindingManifestGeneration(t *testing.T) {
	client := cloudresource.NewCloudResourceClient(resilience.DefaultResilienceConfig())
	ctx := context.Background()

	spec := cloudresource.CloudSQLBindingSpec{
		InstanceID:         "pg-cluster-01",
		DatabaseName:       "payments_ledger",
		AppUser:            "payments_app",
		GSAEmail:           "payments-sa@gov-prod.iam.gserviceaccount.com",
		IAMAuthEnabled:     true,
		DeletionProtection: true,
		Metadata:           defaultMetadata("wl-payments-01"),
	}

	result, err := client.ProvisionCloudSQLBinding(ctx, spec)
	if err != nil {
		t.Fatalf("unexpected error provisioning Cloud SQL binding: %v", err)
	}

	if !strings.Contains(result.ManifestYAML, "CLOUD_IAM_SERVICE_ACCOUNT") {
		t.Errorf("manifest missing CLOUD_IAM_SERVICE_ACCOUNT user type")
	}
	if !strings.Contains(result.ManifestYAML, "payments_ledger") {
		t.Errorf("manifest missing database name")
	}
}

func TestMetadataValidationFailure(t *testing.T) {
	client := cloudresource.NewCloudResourceClient(resilience.DefaultResilienceConfig())
	ctx := context.Background()

	spec := cloudresource.WorkloadIdentityBindingSpec{
		GSAEmail:  "payments-sa@gov-prod.iam.gserviceaccount.com",
		KSAName:   "payments-service",
		Namespace: "payments",
		ProjectID: "gov-prod",
		Metadata: cloudresource.ResourceMetadata{
			RequestID: "",
			Owner:     "secops",
		},
	}

	_, err := client.ProvisionWorkloadIdentity(ctx, spec)
	if err == nil {
		t.Fatalf("expected validation error for empty request_id, got nil")
	}
}

func TestTeardownDeletionProtection(t *testing.T) {
	client := cloudresource.NewCloudResourceClient(resilience.DefaultResilienceConfig())
	ctx := context.Background()

	spec := cloudresource.CloudSQLBindingSpec{
		InstanceID:         "protected-pg",
		DatabaseName:       "audit_log",
		AppUser:            "audit_app",
		DeletionProtection: true,
		Metadata:           defaultMetadata("wl-audit-01"),
	}

	result, err := client.ProvisionCloudSQLBinding(ctx, spec)
	if err != nil {
		t.Fatalf("failed to provision binding: %v", err)
	}

	err = client.TeardownResource(ctx, result.ResourceID, false)
	if err == nil {
		t.Fatalf("expected teardown failure due to deletion protection, got nil")
	}

	err = client.TeardownResource(ctx, result.ResourceID, true)
	if err != nil {
		t.Fatalf("expected successful forced teardown, got error: %v", err)
	}

	rec, err := client.GetResource(ctx, result.ResourceID)
	if err != nil {
		t.Fatalf("failed to retrieve record: %v", err)
	}
	if rec.Status != cloudresource.ResourceStatusTornDown {
		t.Errorf("expected status TORN_DOWN, got %s", rec.Status)
	}
}

func TestLedgerReconciliationOrphans(t *testing.T) {
	client := cloudresource.NewCloudResourceClient(resilience.DefaultResilienceConfig())
	ctx := context.Background()

	spec1 := cloudresource.WorkloadIdentityBindingSpec{
		GSAEmail:  "active-sa@gov-prod.iam.gserviceaccount.com",
		KSAName:   "active-service",
		Namespace: "active",
		ProjectID: "gov-prod",
		Metadata:  defaultMetadata("wl-active-01"),
	}
	spec2 := cloudresource.WorkloadIdentityBindingSpec{
		GSAEmail:  "retired-sa@gov-prod.iam.gserviceaccount.com",
		KSAName:   "retired-service",
		Namespace: "retired",
		ProjectID: "gov-prod",
		Metadata:  defaultMetadata("wl-retired-02"),
	}

	_, _ = client.ProvisionWorkloadIdentity(ctx, spec1)
	_, _ = client.ProvisionWorkloadIdentity(ctx, spec2)

	activeWorkloads := []string{"wl-active-01"}
	report, err := client.ReconcileLedger(ctx, activeWorkloads)
	if err != nil {
		t.Fatalf("failed to reconcile ledger: %v", err)
	}

	if report.TotalManaged != 2 {
		t.Errorf("expected 2 total managed resources, got %d", report.TotalManaged)
	}
	if report.ActiveCount != 1 {
		t.Errorf("expected 1 active resource, got %d", report.ActiveCount)
	}
	if report.OrphanedCount != 1 {
		t.Errorf("expected 1 orphaned resource, got %d", report.OrphanedCount)
	}
	if len(report.OrphanedResourceIDs) != 1 || report.OrphanedResourceIDs[0] != "wi-retired-retired-service" {
		t.Errorf("unexpected orphaned resource IDs: %v", report.OrphanedResourceIDs)
	}
}
