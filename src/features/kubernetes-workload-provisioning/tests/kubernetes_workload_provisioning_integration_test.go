package tests

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	kubernetesworkloadprovisioning "internal-developer-platform/src/features/kubernetes-workload-provisioning"
	k8sclient "internal-developer-platform/src/infra/clients/kubernetes"
	"internal-developer-platform/src/infra/clients/resilience"
)

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
