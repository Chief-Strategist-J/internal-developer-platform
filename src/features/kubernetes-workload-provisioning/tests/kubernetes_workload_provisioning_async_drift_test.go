// Package tests verifies Workload Provisioning consumer-owned AsyncJobDriftPort isolation (X-03, XIII.1).
package tests

import (
	"context"
	"testing"
	"time"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/adapters"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/ports"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/service"
)

type stubAsyncDriftPort struct {
	revoked bool
	reason  string
}

func (s *stubAsyncDriftPort) EvaluateAsyncJobDrift(ctx context.Context, req ports.AsyncJobDriftRequest) (ports.AsyncJobDriftResult, error) {
	return ports.AsyncJobDriftResult{
		Revoked: s.revoked,
		Granted: !s.revoked,
		Reason:  s.reason,
	}, nil
}

func TestWorkloadProvisioning_AsyncJobDriftEnforcedViaConsumerPort(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	k8sClient := adapters.NewKubernetesClientAdapter("https://10.0.0.1:6443")
	svc := service.NewWorkloadProvisioningService(store, k8sClient)
	svc.WithAsyncDriftPort(&stubAsyncDriftPort{revoked: true, reason: "async token drifted past grace window"})

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wld-async-1",
		TenantID:       "tenant-a",
		ServiceID:      "svc-1",
		Namespace:      "ns-a",
		WorkloadName:   "api-a",
		Image:          "gcr.io/idp/api:v1.0.0",
		CPURequest:     "250m",
		CPULimit:       "500m",
		MemoryRequest:  "256Mi",
		MemoryLimit:    "512Mi",
		Replicas:       2,
		IdempotencyKey: "idem-wld-async-1",
	}
	res, err := svc.ProvisionWorkload(ctx, cmd)
	if err != nil {
		t.Fatalf("ProvisionWorkload failed: %v", err)
	}

	asyncCtx := service.WithAsyncJobAuthContext(ctx, service.AsyncJobAuthContext{
		SubjectID:    "worker-1",
		JTI:          "jti-async-1",
		QueueLatency: 45 * time.Minute,
	})
	if _, err := svc.AwaitWorkloadReadiness(asyncCtx, "tenant-a", res.Provision.ID, 50*time.Millisecond); err == nil {
		t.Fatal("expected AwaitWorkloadReadiness to reject when AsyncJobDriftPort reports revoked")
	}
}
