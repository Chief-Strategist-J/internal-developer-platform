// Package bridges: gitops__rbac.bridge wires the gitops-reconciliation feature's
// TaskLeaseGuardPort to the RBAC feature's EvaluateTaskLease capability (X-03).
//
// Blueprint:
//  1. This bridge is the ONLY cross-feature wiring point for task-lease checks.
//  2. gitops-reconciliation calls its own TaskLeaseGuardPort — it never imports rbac directly.
//  3. This bridge adapts the consumer's need-shaped port to the provider's exposed function.
//
// Invariants:
//   - No retry logic here; retries belong in adapters (R-01, VIII.3).
//   - Deadline from the caller's context is passed unchanged.
//   - If RBAC is unavailable, return PortError{Kind: Unavailable} — fail-closed (X-02).
package bridges

import (
	"context"

	"internal-developer-platform/src/features/gitops-reconciliation/infrastructure/ports"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
)

type GitopsRBACBridge struct {
	rbac *rbacService.RBACService
}

func NewGitopsRBACBridge(rbac *rbacService.RBACService) *GitopsRBACBridge {
	return &GitopsRBACBridge{rbac: rbac}
}

func (b *GitopsRBACBridge) EvaluateTaskLease(ctx context.Context, req ports.TaskLeaseRequest) (ports.TaskLeaseResult, error) {
	rbacReq := rbacContracts.TaskExecutionLeaseRequest{
		TokenExpired:          req.TokenExpired,
		HasSignedTaskLease:    req.LeaseToken != "",
		LeaseExpired:          req.LeaseExpired,
		IsIdempotentBatchStep: false,
		RefreshSucceeded:      !req.TokenExpired,
	}
	result, err := b.rbac.EvaluateTaskLease(ctx, rbacReq)
	return ports.TaskLeaseResult{
		Granted:   err == nil && result.Granted,
		Reason:    result.Reason,
		Directive: result.Directive,
	}, err
}
