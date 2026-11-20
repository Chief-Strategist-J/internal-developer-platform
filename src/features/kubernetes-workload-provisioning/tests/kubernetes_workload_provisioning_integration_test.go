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

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	kubernetesworkloadprovisioning "internal-developer-platform/src/features/kubernetes-workload-provisioning"
	paccontracts "internal-developer-platform/src/features/policy-as-code/contracts"
	pacadapters "internal-developer-platform/src/features/policy-as-code/infrastructure/adapters"
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
	adapter := kubernetesworkloadprovisioning.NewPostgresStore(db)

	uniqueSuffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := "tenant-live-pg-" + uniqueSuffix
	recordID := "wkld-live-pg-" + uniqueSuffix
	idempKey := "idemp-live-pg-" + uniqueSuffix

	record := contracts.WorkloadProvisionRecord{
		ID:             recordID,
		PublicID:       "wld_" + uniqueSuffix,
		TenantID:       tenantID,
		Source:         "developer-portal",
		ExternalID:     "ext-" + uniqueSuffix,
		CorrelationID:  "00000000-0000-0000-0000-000000000001",
		ServiceID:      "order-service",
		Namespace:      "orders-live",
		WorkloadName:   "orders-backend",
		CPURequest:     "250m",
		CPULimit:       "500m",
		MemoryRequest:  "256Mi",
		MemoryLimit:    "512Mi",
		Replicas:       2,
		Strategy:         contracts.StrategyCanary,
		Status:           contracts.StatusSubmitted,
		SpecHash:         "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		ApprovedSpecHash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		ApproverRole:     "platform-admin",
		ImagePullPolicy:  "IfNotPresent",
		BreakglassJustification: "SEV-1-hotfix",
		IdempotencyKey:   idempKey,
		Version:          1,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
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
	store := kubernetesworkloadprovisioning.NewMemoryStore()
	k8sClient := kubernetesworkloadprovisioning.NewK8sClient("https://k8s-api.internal:6443")
	svc := kubernetesworkloadprovisioning.NewService(store, k8sClient)

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
	store := kubernetesworkloadprovisioning.NewMemoryStore()
	k8sClient := kubernetesworkloadprovisioning.NewK8sClient("https://k8s-api.internal:6443")
	svc := kubernetesworkloadprovisioning.NewService(store, k8sClient)

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
	adapter := kubernetesworkloadprovisioning.NewPostgresStore(nil)

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

	store := kubernetesworkloadprovisioning.NewMemoryStore()
	k8sAdapter := kubernetesworkloadprovisioning.NewLiveK8sClient(liveClient)
	svc := kubernetesworkloadprovisioning.NewService(store, k8sAdapter)

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

func TestWorkloadProvisioningWithActivePolicyException(t *testing.T) {
	ctx := context.Background()
	store := kubernetesworkloadprovisioning.NewMemoryStore()
	k8sAdapter := kubernetesworkloadprovisioning.NewK8sClient("https://10.0.0.1:6443")
	policyRepo := pacadapters.NewMemoryPolicyRepositoryAdapter()
	svc := kubernetesworkloadprovisioning.NewService(store, k8sAdapter, policyRepo)

	now := time.Now().UTC()
	policy := paccontracts.PolicyRecord{
		ID:               "11111111-2222-3333-4444-555555555555",
		PublicID:         "POL-ROOT-BANNED",
		TenantID:         "tenant-exc-test",
		Source:           paccontracts.PolicySourceSystem,
		ExternalID:       "EXT-ROOT-001",
		CorrelationID:    "66666666-7777-8888-9999-000000000000",
		Name:             "Restricted Pod Security Root Ban",
		EnforcementPoint: paccontracts.EnforcementPointCEL,
		Severity:         paccontracts.SeverityCritical,
		LifecycleState:   paccontracts.LifecycleDeny,
		RuleExpression:   "!c.securityContext.privileged",
		Exceptable:       true,
		DetectionPair:    "DET-01",
		Version:          1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := policyRepo.SavePolicy(ctx, policy); err != nil {
		t.Fatalf("failed to save policy: %v", err)
	}

	exc := paccontracts.PolicyExceptionRecord{
		ID:            "99999999-8888-7777-6666-555555555555",
		PublicID:      "EXC-LEGACY-ROOT-001",
		TenantID:      "tenant-exc-test",
		PolicyID:      policy.ID,
		WorkloadID:    "legacy-daemon",
		Namespace:     "tenant-exc-test",
		ApproverID:    "secops-lead@internal",
		Justification: "Migration waiver granted until Q3 for legacy daemon",
		Status:        paccontracts.ExceptionStatusActive,
		ExpiresAt:     now.Add(48 * time.Hour),
		CorrelationID: "66666666-7777-8888-9999-000000000000",
		Version:       1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := policyRepo.SaveException(ctx, exc); err != nil {
		t.Fatalf("failed to save policy exception: %v", err)
	}

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
	kubernetesworkloadprovisioning.ClientPort
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
	store := kubernetesworkloadprovisioning.NewMemoryStore()
	k8sClient := kubernetesworkloadprovisioning.NewK8sClient("https://k8s-api.internal:6443")
	committer := gitops.NewGitManifestCommitter(nil)
	gitopsAdapter := kubernetesworkloadprovisioning.NewGitOpsDelivery(committer, "main", "idp-bot", "idp@company.com")
	svc := kubernetesworkloadprovisioning.NewService(store, k8sClient).WithGitOpsDelivery(gitopsAdapter)

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
	store := kubernetesworkloadprovisioning.NewMemoryStore()
	rawK8sClient := kubernetesworkloadprovisioning.NewK8sClient("https://k8s-api.internal:6443")
	delayedClient := &delayedK8sPort{ClientPort: rawK8sClient, ready: false}
	svc := kubernetesworkloadprovisioning.NewService(store, delayedClient)

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
	store := kubernetesworkloadprovisioning.NewMemoryStore()
	rawK8sClient := kubernetesworkloadprovisioning.NewK8sClient("https://k8s-api.internal:6443")
	neverReadyClient := &delayedK8sPort{ClientPort: rawK8sClient, ready: false}
	svc := kubernetesworkloadprovisioning.NewService(store, neverReadyClient)

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
	store := kubernetesworkloadprovisioning.NewMemoryStore()
	k8sClient := kubernetesworkloadprovisioning.NewK8sClient("https://k8s-api.internal:6443")
	svc := kubernetesworkloadprovisioning.NewService(store, k8sClient)

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
	store := kubernetesworkloadprovisioning.NewMemoryStore()
	k8sClient := kubernetesworkloadprovisioning.NewK8sClient("https://k8s-api.internal:6443")
	svc := kubernetesworkloadprovisioning.NewService(store, k8sClient)

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
	store := kubernetesworkloadprovisioning.NewMemoryStore()
	k8sClient := kubernetesworkloadprovisioning.NewK8sClient("https://k8s-api.internal:6443")
	svc := kubernetesworkloadprovisioning.NewService(store, k8sClient)

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
	store := kubernetesworkloadprovisioning.NewMemoryStore()
	k8sClient := kubernetesworkloadprovisioning.NewK8sClient("https://k8s-api.internal:6443")
	committer := gitops.NewGitManifestCommitter(nil)
	gitopsAdapter := kubernetesworkloadprovisioning.NewGitOpsDelivery(committer, "main", "idp-bot", "idp@company.com")
	svc := kubernetesworkloadprovisioning.NewService(store, k8sClient).WithGitOpsDelivery(gitopsAdapter)

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
	store := kubernetesworkloadprovisioning.NewMemoryStore()
	k8sClient := kubernetesworkloadprovisioning.NewK8sClient("https://k8s-api.internal:6443")
	mockKMS := kms.NewMockKMSClient()
	kmsKeyURI := "projects/platform-security-kms/locations/global/keyRings/ring/cryptoKeys/key"
	_ = mockKMS.RegisterKey(kmsKeyURI, make([]byte, 32))
	engine := kms.NewEnvelopeEncryptionEngine(mockKMS)
	svc := kubernetesworkloadprovisioning.NewService(store, k8sClient).WithKMSEngine(engine)

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



