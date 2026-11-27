// Package tests verifies GitOps Reconciliation consumer-owned TaskLeaseGuardPort isolation (X-03, XIII.1).
package tests

import (
	"context"
	"testing"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/infrastructure/adapters"
	"internal-developer-platform/src/features/gitops-reconciliation/infrastructure/ports"
	"internal-developer-platform/src/features/gitops-reconciliation/service"
)

type stubTaskLeasePort struct {
	granted bool
	reason  string
}

func (s *stubTaskLeasePort) EvaluateTaskLease(ctx context.Context, req ports.TaskLeaseRequest) (ports.TaskLeaseResult, error) {
	return ports.TaskLeaseResult{Granted: s.granted, Reason: s.reason}, nil
}

func TestGitOpsReconciliation_TaskLeaseGuardEnforcedViaConsumerPort(t *testing.T) {
	ctx := context.Background()
	store := adapters.NewMemoryGitOpsRepositoryAdapter()
	argo := adapters.NewArgoCDClientAdapter("http://localhost:8085", "tok")
	svc := service.NewGitOpsReconciliationService(store, argo)
	svc.SetTaskLeasePort(&stubTaskLeasePort{granted: false, reason: "lease expired for non-idempotent task"})

	cmd := contracts.SyncCommand{
		ID:                 "rec-lease-block-1",
		TenantID:           "tenant-a",
		ServiceID:          "svc-1",
		AppName:            "app-1",
		TargetRevision:     "v1.0.0",
		ProjectEnforcesSig: true,
		CommitSigned:       true,
		AllowedSigningKey:  true,
		RoleReviewed:       true,
		InAllowWindow:      true,
		IdempotencyKey:     "idem-lease-1",
		TaskLeaseRequired:  true,
		TokenExpired:       true,
		LeaseExpired:       true,
	}

	if _, err := svc.TriggerReconciliation(ctx, cmd); err == nil {
		t.Fatal("expected TriggerReconciliation to deny sync when TaskLeaseGuardPort denies lease")
	}

	svc.SetTaskLeasePort(&stubTaskLeasePort{granted: true, reason: "valid lease"})
	cmd.ID = "rec-lease-allow-1"
	cmd.IdempotencyKey = "idem-lease-2"
	cmd.TokenExpired = false
	cmd.LeaseExpired = false
	if _, err := svc.TriggerReconciliation(ctx, cmd); err != nil {
		t.Fatalf("expected TriggerReconciliation to succeed with granted task lease, got %v", err)
	}
}
