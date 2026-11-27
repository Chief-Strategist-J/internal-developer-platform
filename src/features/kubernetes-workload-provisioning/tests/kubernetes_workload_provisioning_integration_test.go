// Package tests provides infrastructure integration tests for Kubernetes Workload Provisioning.
//
// Architectural Blueprint:
// - Step 14 Integration Test: Verifies concrete outbound infrastructure adapters.
// - Tests Postgres store, in-memory store, Kubernetes client adapter, GitOps delivery, and KMS encryption.
// - 100% comment-free function bodies.
package tests

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	_ "github.com/lib/pq"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/internal/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/internal/domain/rules"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/internal/infrastructure/ports"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/internal/service"
	"internal-developer-platform/src/infra/clients/gitops"
	"internal-developer-platform/src/infra/clients/kms"
	k8sclient "internal-developer-platform/src/infra/clients/kubernetes"
	"internal-developer-platform/src/infra/clients/resilience"
	"internal-developer-platform/src/infra/observability"
)

func TestMain(m *testing.M) {
	_, shutdown, _ := observability.InitTracer("kubernetes-workload-provisioning-test")
	code := m.Run()
	if shutdown != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = shutdown(ctx)
		cancel()
	}
	os.Exit(code)
}

func getTestDatabaseConnection(t *testing.T) *sql.DB {
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("DB_PORT")
	if port == "" {
		port = "5432"
	}
	user := os.Getenv("DB_USER")
	if user == "" {
		user = "postgres"
	}
	pass := os.Getenv("DB_PASSWORD")
	if pass == "" {
		pass = "postgres_dev_password_123"
	}
	dbname := os.Getenv("DB_NAME")
	if dbname == "" {
		dbname = "idp_db"
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable connect_timeout=3", host, port, user, pass, dbname)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("skipping live postgres tests: %v", err)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("skipping live postgres tests: database not reachable: %v", err)
		return nil
	}
	return db
}

func TestPostgresWorkloadRepositoryLive(t *testing.T) {
	db := getTestDatabaseConnection(t)
	if db == nil {
		return
	}
	defer db.Close()

	ctx := context.Background()
	adapter := adapters.NewPostgresWorkloadRepositoryAdapter(db)

	uniqueSuffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := "tenant-live-pg-" + uniqueSuffix
	recordID := "wkld-live-pg-" + uniqueSuffix
	idempKey := "idemp-live-pg-" + uniqueSuffix

	record := contracts.WorkloadProvisionRecord{
		ID:                      recordID,
		PublicID:                "wld_" + uniqueSuffix,
		TenantID:                tenantID,
		Source:                  "developer-portal",
		ExternalID:              "ext-" + uniqueSuffix,
		CorrelationID:           "00000000-0000-0000-0000-000000000001",
		ServiceID:               "order-service",
		Namespace:               "orders-live",
		WorkloadName:            "orders-backend",
		CPURequest:              "250m",
		CPULimit:                "500m",
		MemoryRequest:           "256Mi",
		MemoryLimit:             "512Mi",
		Replicas:                2,
		Strategy:                contracts.StrategyCanary,
		Status:                  contracts.StatusSubmitted,
		SpecHash:                "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		ApprovedSpecHash:        "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		ApproverRole:            "platform-admin",
		ImagePullPolicy:         "IfNotPresent",
		BreakglassJustification: "SEV-1-hotfix",
		IdempotencyKey:          idempKey,
		Version:                 1,
		CreatedAt:               time.Now().UTC(),
		UpdatedAt:               time.Now().UTC(),
	}

	if err := adapter.Save(ctx, record); err != nil {
		t.Fatalf("failed to insert record into live postgres: %v", err)
	}

	fetched, err := adapter.GetByID(ctx, tenantID, recordID)
	if err != nil {
		t.Fatalf("failed to fetch record by id: %v", err)
	}
	if fetched.WorkloadName != "orders-backend" {
		t.Errorf("expected WorkloadName orders-backend, got %s", fetched.WorkloadName)
	}
	if fetched.SpecHash != record.SpecHash {
		t.Errorf("expected SpecHash %s, got %s", record.SpecHash, fetched.SpecHash)
	}
	if fetched.ApprovedSpecHash != record.ApprovedSpecHash {
		t.Errorf("expected ApprovedSpecHash %s, got %s", record.ApprovedSpecHash, fetched.ApprovedSpecHash)
	}
	if fetched.ApproverRole != record.ApproverRole {
		t.Errorf("expected ApproverRole %s, got %s", record.ApproverRole, fetched.ApproverRole)
	}
	if fetched.ImagePullPolicy != record.ImagePullPolicy {
		t.Errorf("expected ImagePullPolicy %s, got %s", record.ImagePullPolicy, fetched.ImagePullPolicy)
	}
	if fetched.BreakglassJustification != record.BreakglassJustification {
		t.Errorf("expected BreakglassJustification %s, got %s", record.BreakglassJustification, fetched.BreakglassJustification)
	}
	if fetched.PublicID != "wld_"+uniqueSuffix {
		t.Errorf("expected PublicID wld_%s, got %s", uniqueSuffix, fetched.PublicID)
	}
	if fetched.Source != "developer-portal" {
		t.Errorf("expected Source developer-portal, got %s", fetched.Source)
	}
	if fetched.ExternalID != "ext-"+uniqueSuffix {
		t.Errorf("expected ExternalID ext-%s, got %s", uniqueSuffix, fetched.ExternalID)
	}
	if fetched.CorrelationID != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("expected CorrelationID 00000000-0000-0000-0000-000000000001, got %s", fetched.CorrelationID)
	}
	if fetched.Strategy != contracts.StrategyCanary {
		t.Errorf("expected Strategy CANARY, got %s", fetched.Strategy)
	}

	byKey, err := adapter.GetByIdempotencyKey(ctx, tenantID, idempKey)
	if err != nil {
		t.Fatalf("failed to fetch record by idempotency key: %v", err)
	}
	if byKey == nil || byKey.ID != recordID {
		t.Fatalf("expected record id %s by idempotency key, got %+v", recordID, byKey)
	}

	list, err := adapter.List(ctx, contracts.FilterCriteria{TenantID: tenantID, Limit: 10})
	if err != nil {
		t.Fatalf("failed to list records: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 record in list for tenant, got %d", len(list))
	}

	record.Status = contracts.StatusActive
	if err := adapter.Update(ctx, record); err != nil {
		t.Fatalf("failed to update record: %v", err)
	}

	updated, err := adapter.GetByID(ctx, tenantID, recordID)
	if err != nil {
		t.Fatalf("failed to fetch updated record: %v", err)
	}
	if updated.Status != contracts.StatusActive {
		t.Errorf("expected status ACTIVE after update, got %s", updated.Status)
	}

	staleRecord := record
	staleRecord.Version = 1
	if err := adapter.Update(ctx, staleRecord); err == nil {
		t.Fatalf("expected optimistic locking conflict on stale update version")
	}
}

func TestWorkloadProvisioningServiceExecution(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	k8sClient := adapters.NewKubernetesClientAdapter("https://k8s-api.internal:6443")
	svc := service.NewWorkloadProvisioningService(store, k8sClient)

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-test-1",
		TenantID:       "tenant-qa",
		ServiceID:      "auth-service",
		Namespace:      "auth-qa",
		WorkloadName:   "auth-deploy",
		Image:          "repo/auth:latest",
		CPURequest:     "",
		CPULimit:       "",
		MemoryRequest:  "",
		MemoryLimit:    "",
		Replicas:       3,
		IdempotencyKey: "idemp-wkld-001",
	}

	result, err := svc.ProvisionWorkload(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error provisioning workload: %v", err)
	}

	if result.Provision.Status != contracts.StatusObservedReady && result.Provision.Status != contracts.StatusActive {
		t.Errorf("expected status OBSERVED_READY or ACTIVE, got %s", result.Provision.Status)
	}
	if result.Provision.CPURequest != "250m" {
		t.Errorf("expected default injected CPURequest 250m, got %s", result.Provision.CPURequest)
	}
	if !result.PDBCreated {
		t.Errorf("expected PDB created for replicas=3")
	}
	if result.Manifests.HTTPRouteYAML == "" {
		t.Errorf("expected HTTPRouteYAML in synthesized manifests")
	}
	if result.Manifests.BackendPolicyYAML == "" {
		t.Errorf("expected BackendPolicyYAML in synthesized manifests")
	}

	replay, err := svc.ProvisionWorkload(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error on idempotent replay: %v", err)
	}
	if replay.Message != "idempotent replay" {
		t.Errorf("expected idempotent replay message, got: %s", replay.Message)
	}

	fetched, err := svc.GetWorkload(ctx, "tenant-qa", "wkld-test-1")
	if err != nil {
		t.Fatalf("unexpected error fetching record: %v", err)
	}
	if fetched.ID != "wkld-test-1" {
		t.Errorf("expected ID wkld-test-1, got %s", fetched.ID)
	}

	list, err := svc.ListWorkloads(ctx, contracts.FilterCriteria{TenantID: "tenant-qa"})
	if err != nil {
		t.Fatalf("unexpected error listing records: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 record in list, got %d", len(list))
	}
}

func TestWorkloadProvisioningQuotaExceeded(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	k8sClient := adapters.NewKubernetesClientAdapter("https://k8s-api.internal:6443")
	svc := service.NewWorkloadProvisioningService(store, k8sClient)

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-heavy-01",
		TenantID:       "tenant-qa",
		ServiceID:      "heavy-service",
		Namespace:      "default",
		WorkloadName:   "heavy-deploy",
		Image:          "repo/heavy:1.0.0",
		CPURequest:     "8000m",
		CPULimit:       "16000m",
		MemoryRequest:  "16Gi",
		MemoryLimit:    "32Gi",
		Replicas:       5,
		IdempotencyKey: "idemp-heavy-01",
	}

	_, err := svc.ProvisionWorkload(ctx, cmd)
	if err == nil {
		t.Fatalf("expected error due to quota headroom exhaustion")
	}
	if !strings.Contains(err.Error(), "insufficient quota headroom") {
		t.Errorf("expected insufficient quota headroom in error, got: %v", err)
	}
}

func TestPostgresAdapterValidationGuards(t *testing.T) {
	ctx := context.Background()
	adapter := adapters.NewPostgresWorkloadRepositoryAdapter(nil)

	err := adapter.Save(ctx, contracts.WorkloadProvisionRecord{})
	if err == nil {
		t.Fatalf("expected validation error for empty record")
	}

	_, err = adapter.GetByID(ctx, "", "wkld-1")
	if err == nil {
		t.Fatalf("expected validation error for empty tenantId")
	}

	_, err = adapter.GetByIdempotencyKey(ctx, "tenant-qa", "")
	if err == nil {
		t.Fatalf("expected validation error for empty idempotency key")
	}

	_, err = adapter.List(ctx, contracts.FilterCriteria{})
	if err == nil {
		t.Fatalf("expected validation error for empty tenant in list")
	}

	err = adapter.Update(ctx, contracts.WorkloadProvisionRecord{})
	if err == nil {
		t.Fatalf("expected validation error for empty record in update")
	}
}

func TestWorkloadProvisioningServiceWithLiveK8sClient(t *testing.T) {
	ctx := context.Background()
	fakeClientset := fake.NewSimpleClientset()
	resilienceCfg := resilience.DefaultResilienceConfig()
	liveClient := k8sclient.NewKubernetesClient(fakeClientset, resilienceCfg)

	quota := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "compute-quota",
			Namespace: "tenant-live",
		},
		Status: corev1.ResourceQuotaStatus{
			Hard: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("20"),
				corev1.ResourceMemory: resource.MustParse("32Gi"),
			},
			Used: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	}
	_, err := liveClient.CreateOrUpdateResourceQuota(ctx, "tenant-live", quota)
	if err != nil {
		t.Fatalf("failed to seed resource quota in fake k8s client: %v", err)
	}

	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	k8sAdapter := adapters.NewLiveKubernetesClientAdapter(liveClient)
	svc := service.NewWorkloadProvisioningService(store, k8sAdapter)

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-live-1",
		TenantID:       "tenant-live",
		ServiceID:      "order-service",
		Namespace:      "tenant-live",
		WorkloadName:   "order-deploy",
		Image:          "repo/orders:v1.0.0",
		CPURequest:     "500m",
		CPULimit:       "1000m",
		MemoryRequest:  "512Mi",
		MemoryLimit:    "1024Mi",
		Replicas:       2,
		Strategy:       contracts.StrategyCanary,
		IdempotencyKey: "idemp-live-001",
	}

	result, err := svc.ProvisionWorkload(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error provisioning with live k8s client: %v", err)
	}

	if result.Provision.Namespace != "tenant-live" {
		t.Fatalf("expected namespace tenant-live, got %s", result.Provision.Namespace)
	}
	if result.Provision.Strategy != contracts.StrategyCanary {
		t.Fatalf("expected strategy Canary, got %s", result.Provision.Strategy)
	}
}

type stubPolicyExceptionGuard struct {
	exception *ports.ActivePolicyException
}

func (s *stubPolicyExceptionGuard) GetActiveExceptionForWorkload(ctx context.Context, tenantID, namespace, workloadName string) (*ports.ActivePolicyException, error) {
	return s.exception, nil
}

func TestWorkloadProvisioningWithActivePolicyException(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	k8sAdapter := adapters.NewKubernetesClientAdapter("https://10.0.0.1:6443")
	svc := service.NewWorkloadProvisioningService(store, k8sAdapter, &stubPolicyExceptionGuard{
		exception: &ports.ActivePolicyException{
			PublicID:   "EXC-LEGACY-ROOT-001",
			ApproverID: "secops-lead@internal",
		},
	})

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-exc-001",
		TenantID:       "tenant-exc-test",
		ServiceID:      "legacy-service",
		Namespace:      "tenant-exc-test",
		WorkloadName:   "legacy-daemon",
		Image:          "gcr.io/corp/legacy:v1.0.0",
		CPURequest:     "500m",
		CPULimit:       "1000m",
		MemoryRequest:  "512Mi",
		MemoryLimit:    "1024Mi",
		Replicas:       2,
		Strategy:       contracts.StrategyCanary,
		IdempotencyKey: "idemp-exc-001",
	}

	res, err := svc.ProvisionWorkload(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error provisioning workload: %v", err)
	}
	if !res.Admission.Admitted {
		t.Fatalf("expected workload to be admitted via exception, got admitted=false")
	}
}

type delayedK8sPort struct {
	ports.K8sClientPort
	mu    sync.Mutex
	ready bool
}

func (d *delayedK8sPort) ObserveWorkloadReadiness(ctx context.Context, ns, name string, desired int) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ready, nil
}

func (d *delayedK8sPort) setReady(ready bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.ready = ready
}

func TestWorkloadProvisioningWithGitOpsDelivery(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	k8sClient := adapters.NewKubernetesClientAdapter("https://k8s-api.internal:6443")
	committer := gitops.NewGitManifestCommitter(nil)
	gitopsAdapter := adapters.NewGitOpsDeliveryAdapter(committer, "main", "idp-bot", "idp@company.com")
	svc := service.NewWorkloadProvisioningService(store, k8sClient).WithGitOpsDelivery(gitopsAdapter)

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-gitops-01",
		TenantID:       "tenant-gitops",
		ServiceID:      "gitops-svc",
		Namespace:      "gitops-ns",
		WorkloadName:   "gitops-deploy",
		Image:          "gcr.io/company/gitops:v1.0.0",
		Replicas:       2,
		IdempotencyKey: "idemp-gitops-01",
	}

	result, err := svc.ProvisionWorkload(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error provisioning with GitOps: %v", err)
	}
	if result.Provision.ID != "wkld-gitops-01" {
		t.Errorf("expected ID wkld-gitops-01, got %s", result.Provision.ID)
	}
	if result.Manifests.DeploymentYAML == "" {
		t.Errorf("expected DeploymentYAML in synthesized manifests")
	}
}

func TestWorkloadProvisioningAsyncReadinessLifecycle(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	rawK8sClient := adapters.NewKubernetesClientAdapter("https://k8s-api.internal:6443")
	delayedClient := &delayedK8sPort{K8sClientPort: rawK8sClient, ready: false}
	svc := service.NewWorkloadProvisioningService(store, delayedClient)

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-async-01",
		TenantID:       "tenant-async",
		ServiceID:      "async-svc",
		Namespace:      "async-ns",
		WorkloadName:   "async-deploy",
		Image:          "gcr.io/company/async:v1.0.0",
		Replicas:       2,
		IdempotencyKey: "idemp-async-01",
	}

	result, err := svc.ProvisionWorkload(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error on initial provision: %v", err)
	}
	if result.Provision.Status != contracts.StatusProvisioning {
		t.Errorf("expected synchronous status to remain PROVISIONING, got %s", result.Provision.Status)
	}

	go func() {
		time.Sleep(30 * time.Millisecond)
		delayedClient.setReady(true)
	}()

	observed, err := svc.AwaitWorkloadReadiness(ctx, cmd.TenantID, cmd.ID, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("unexpected error waiting for readiness: %v", err)
	}
	if observed.Status != contracts.StatusObservedReady {
		t.Errorf("expected OBSERVED_READY, got %s", observed.Status)
	}
}

func TestWorkloadProvisioningReadinessTimeoutDegrades(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	rawK8sClient := adapters.NewKubernetesClientAdapter("https://k8s-api.internal:6443")
	neverReadyClient := &delayedK8sPort{K8sClientPort: rawK8sClient, ready: false}
	svc := service.NewWorkloadProvisioningService(store, neverReadyClient)

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-timeout-01",
		TenantID:       "tenant-timeout",
		ServiceID:      "timeout-svc",
		Namespace:      "timeout-ns",
		WorkloadName:   "timeout-deploy",
		Image:          "gcr.io/company/timeout:v1.0.0",
		Replicas:       2,
		IdempotencyKey: "idemp-timeout-01",
	}

	result, err := svc.ProvisionWorkload(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error on initial provision: %v", err)
	}
	if result.Provision.Status != contracts.StatusProvisioning {
		t.Errorf("expected synchronous status PROVISIONING, got %s", result.Provision.Status)
	}

	observed, err := svc.AwaitWorkloadReadiness(ctx, cmd.TenantID, cmd.ID, 60*time.Millisecond)
	if err == nil {
		t.Fatalf("expected deadline exceeded error, got nil")
	}
	if observed.Status != contracts.StatusDegraded {
		t.Errorf("expected status DEGRADED upon timeout, got %s", observed.Status)
	}
}

func TestWorkloadProvisioningSpecHashTamperingRejected(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	k8sClient := adapters.NewKubernetesClientAdapter("https://k8s-api.internal:6443")
	svc := service.NewWorkloadProvisioningService(store, k8sClient)

	cmd := contracts.WorkloadProvisionCommand{
		ID:               "wkld-tamper-01",
		TenantID:         "tenant-qa",
		ServiceID:        "auth-service",
		Namespace:        "auth-qa",
		WorkloadName:     "auth-deploy",
		Image:            "repo/auth:v2.0.0",
		Replicas:         2,
		ApprovedSpecHash: "deadbeef00000000000000000000000000000000000000000000000000000000",
		IdempotencyKey:   "idemp-tamper-01",
	}

	_, err := svc.ProvisionWorkload(ctx, cmd)
	if err == nil || !strings.Contains(err.Error(), "spec edited during approval") {
		t.Fatalf("expected spec hash mismatch error, got: %v", err)
	}
}

func TestWorkloadProvisioningProductionNamespaceApprovalGate(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	k8sClient := adapters.NewKubernetesClientAdapter("https://k8s-api.internal:6443")
	svc := service.NewWorkloadProvisioningService(store, k8sClient)

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-prod-gate-01",
		TenantID:       "tenant-prod",
		ServiceID:      "payment-service",
		Namespace:      "payments-prod",
		WorkloadName:   "payment-api",
		Image:          "repo/pay:v1.0.0@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Replicas:       2,
		IdempotencyKey: "idemp-prod-gate-01",
	}

	_, err := svc.ProvisionWorkload(ctx, cmd)
	if err == nil || !strings.Contains(err.Error(), "require explicit approval") {
		t.Fatalf("expected production approval gate error, got: %v", err)
	}

	cmd.ApprovedSpecHash = contracts.ComputeWorkloadSpecHash(cmd)
	cmd.ApproverRole = "secops-lead"
	res, err := svc.ProvisionWorkload(ctx, cmd)
	if err != nil {
		t.Fatalf("expected provisioning to succeed with approval, got: %v", err)
	}
	if res.Provision.ApprovedSpecHash != cmd.ApprovedSpecHash {
		t.Errorf("expected saved record to carry approved spec hash")
	}
	if res.Provision.ImagePullPolicy != "IfNotPresent" {
		t.Errorf("expected ImagePullPolicy IfNotPresent, got %s", res.Provision.ImagePullPolicy)
	}
}

func TestWorkloadProvisioningProductionBreakglassBypass(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	k8sClient := adapters.NewKubernetesClientAdapter("https://k8s-api.internal:6443")
	svc := service.NewWorkloadProvisioningService(store, k8sClient)

	cmd := contracts.WorkloadProvisionCommand{
		ID:                      "wkld-brk-01",
		TenantID:                "tenant-prod",
		ServiceID:               "payment-service",
		Namespace:               "payments-prod",
		WorkloadName:            "payment-api",
		Image:                   "repo/pay:v1.0.0",
		BreakglassJustification: "SEV-1 emergency mitigation approved by security on-call",
		Replicas:                2,
		IdempotencyKey:          "idemp-brk-01",
	}
	cmd.ApprovedSpecHash = contracts.ComputeWorkloadSpecHash(cmd)
	cmd.ApproverRole = "incident-commander"

	res, err := svc.ProvisionWorkload(ctx, cmd)
	if err != nil {
		t.Fatalf("expected breakglass deployment to succeed, got: %v", err)
	}
	if res.Provision.BreakglassJustification != cmd.BreakglassJustification {
		t.Errorf("expected breakglass justification to be preserved in record")
	}
	if !res.Admission.Admitted {
		t.Errorf("expected admission to be admitted via breakglass")
	}
}

func TestWorkloadProvisioningRollbackRevertsGitAndDegradesState(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	k8sClient := adapters.NewKubernetesClientAdapter("https://k8s-api.internal:6443")
	committer := gitops.NewGitManifestCommitter(nil)
	gitopsAdapter := adapters.NewGitOpsDeliveryAdapter(committer, "main", "idp-bot", "idp@company.com")
	svc := service.NewWorkloadProvisioningService(store, k8sClient).WithGitOpsDelivery(gitopsAdapter)

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-rollback-01",
		TenantID:       "tenant-rollback",
		ServiceID:      "auth-service",
		Namespace:      "auth-qa",
		WorkloadName:   "auth-deploy",
		Image:          "repo/auth:v1.0.0",
		Replicas:       2,
		IdempotencyKey: "idemp-rollback-01",
	}

	result, err := svc.ProvisionWorkload(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error provisioning: %v", err)
	}

	rolledBack, err := svc.RollbackWorkload(ctx, cmd.TenantID, result.Provision.ID, "Canary analysis failed: error delta exceeded")
	if err != nil {
		t.Fatalf("unexpected error during rollback: %v", err)
	}
	if rolledBack.Status != contracts.StatusDegraded {
		t.Errorf("expected DEGRADED status upon rollback, got %s", rolledBack.Status)
	}
}

func TestWorkloadProvisioningWithKMSEnvelopeEncryption(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	k8sClient := adapters.NewKubernetesClientAdapter("https://k8s-api.internal:6443")
	mockKMS := kms.NewMockKMSClient()
	kmsKeyURI := "projects/platform-security-kms/locations/global/keyRings/ring/cryptoKeys/key"
	_ = mockKMS.RegisterKey(kmsKeyURI, make([]byte, 32))
	engine := kms.NewEnvelopeEncryptionEngine(mockKMS)
	svc := service.NewWorkloadProvisioningService(store, k8sClient).WithKMSPort(adapters.NewKMSEncryptionAdapter(engine))

	cmd := contracts.WorkloadProvisionCommand{
		ID:               "wkld-kms-01",
		TenantID:         "tenant-sec",
		ServiceID:        "secret-service",
		Namespace:        "sec-prod",
		WorkloadName:     "vault-client",
		Image:            "gcr.io/bank/sec@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Replicas:         2,
		IdempotencyKey:   "idemp-kms-01",
		KMSKeyURI:        kmsKeyURI,
		EncryptedSecrets: map[string]string{"DB_PASSWORD": "super-secret-password"},
	}
	hash := contracts.ComputeWorkloadSpecHash(cmd)
	cmd.ApprovedSpecHash = hash
	cmd.ApproverRole = "secops-lead"

	res, err := svc.ProvisionWorkload(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error provisioning workload with kms: %v", err)
	}
	if !res.Admission.Admitted {
		t.Errorf("expected admitted workload")
	}
	if res.Manifests.SecretYAML == "" {
		t.Errorf("expected Secret manifest to be generated")
	}
	if !strings.Contains(res.Manifests.SecretYAML, "AES-256-GCM-KMS-ENVELOPE") {
		t.Errorf("expected envelope encryption algorithm annotation in Secret YAML")
	}
}

func TestWorkloadProvisioning_RejectsPermissiveMtlsInProduction(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	k8sClient := adapters.NewKubernetesClientAdapter("https://k8s-api.internal:6443")
	svc := service.NewWorkloadProvisioningService(store, k8sClient)

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-mesh-01",
		TenantID:       "tenant-mesh",
		ServiceID:      "mesh-service",
		Namespace:      "mesh-prod",
		WorkloadName:   "mesh-workload",
		Image:          "gcr.io/bank/mesh@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Replicas:       2,
		IdempotencyKey: "idemp-mesh-01",
		MeshSecurity:   contracts.MeshSecurityConfig{MtlsMode: "PERMISSIVE"},
	}
	hash := contracts.ComputeWorkloadSpecHash(cmd)
	cmd.ApprovedSpecHash = hash
	cmd.ApproverRole = "platform-admin"

	_, err := svc.ProvisionWorkload(ctx, cmd)
	if err == nil {
		t.Fatalf("expected error when provisioning prod workload with PERMISSIVE mtls")
	}
	if !strings.Contains(err.Error(), "critical security violations") {
		t.Errorf("expected critical security violation, got: %v", err)
	}
}

func TestWorkloadProvisioning_AuditLoggingCompliance(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	k8sClient := adapters.NewKubernetesClientAdapter("https://k8s-api.internal:6443")
	svc := service.NewWorkloadProvisioningService(store, k8sClient)

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-audit-01",
		TenantID:       "tenant-audit",
		ServiceID:      "audit-service",
		Namespace:      "audit-prod",
		WorkloadName:   "audit-workload",
		Image:          "gcr.io/bank/audit@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Replicas:       2,
		IdempotencyKey: "idemp-audit-01",
		AuditLogging: contracts.AuditLoggingConfig{
			EnableDataAccessLogs: true,
			DestinationBucket:    "audit-archive-bucket",
			RetentionDays:        400,
			LockedRetention:      true,
			OffNodeLogCollection: true,
			PIIScrubbingEnabled:  true,
		},
	}
	hash := contracts.ComputeWorkloadSpecHash(cmd)
	cmd.ApprovedSpecHash = hash
	cmd.ApproverRole = "compliance-lead"

	res, err := svc.ProvisionWorkload(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error provisioning workload with audit logging: %v", err)
	}
	if !res.Admission.Admitted {
		t.Errorf("expected admitted workload")
	}
	if res.Provision.AuditDestinationBucket != "audit-archive-bucket" {
		t.Errorf("expected AuditDestinationBucket audit-archive-bucket, got %s", res.Provision.AuditDestinationBucket)
	}
	if !res.Provision.AuditLockedRetention {
		t.Errorf("expected AuditLockedRetention true in persisted record")
	}
}

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

func TestGenerateManifestBundle_AuditLogSinkAndBucket(t *testing.T) {
	ctx := context.Background()
	client := adapters.NewKubernetesClientAdapter("https://10.0.0.1:6443")
	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-manifest-audit-01",
		TenantID:       "tenant-payments",
		ServiceID:      "payment-api",
		Namespace:      "payments-prod",
		WorkloadName:   "payment-processor",
		Image:          "gcr.io/bank/payment@sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Replicas:       2,
		IdempotencyKey: "idemp-manifest-audit-01",
		AuditLogging: contracts.AuditLoggingConfig{
			EnableDataAccessLogs: true,
			DestinationBucket:    "payments-audit-sink-bucket",
			RetentionDays:        400,
			LockedRetention:      true,
			OffNodeLogCollection: true,
			PIIScrubbingEnabled:  true,
		},
	}

	mutations := rules.EvaluateWorkloadMutations(cmd)
	bundle, err := client.GenerateManifestBundle(ctx, cmd, mutations)
	if err != nil {
		t.Fatalf("unexpected error generating manifest bundle: %v", err)
	}
	if !strings.Contains(bundle.LogSinkYAML, "kind: LoggingLogSink") {
		t.Errorf("expected LoggingLogSink manifest in bundle")
	}
	if !strings.Contains(bundle.LogSinkYAML, "payments-audit-sink-bucket") {
		t.Errorf("expected destination bucket in LogSink")
	}
	if !strings.Contains(bundle.LoggingBucketYAML, "kind: StorageBucket") {
		t.Errorf("expected StorageBucket manifest in bundle")
	}
	if !strings.Contains(bundle.LoggingBucketYAML, "isLocked: true") {
		t.Errorf("expected isLocked: true in StorageBucket")
	}
}
