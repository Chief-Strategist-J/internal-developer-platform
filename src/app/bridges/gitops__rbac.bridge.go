// Package bridges: gitops__rbac.bridge wires the gitops-reconciliation feature's
// TaskLeaseBridge to the RBAC feature's EvaluateTaskLease capability via index.go only (X-03, GO-INT-001).
//
// Blueprint:
//  1. Zero internal/ imports — consumes rbac.RBACBridgeProvider and implements gitopsreconciliation.TaskLeaseBridge.
//  2. No retry logic or business rules in the bridge (R-01).
package bridges

import (
	"context"

	gitopsreconciliation "internal-developer-platform/src/features/gitops-reconciliation"
	rbac "internal-developer-platform/src/features/role-based-access-control"
)

type GitopsRBACBridge struct {
	rbac rbac.RBACBridgeProvider
}

func NewGitopsRBACBridge(rbacProvider rbac.RBACBridgeProvider) *GitopsRBACBridge {
	return &GitopsRBACBridge{rbac: rbacProvider}
}

func (b *GitopsRBACBridge) EvaluateTaskLease(ctx context.Context, req gitopsreconciliation.TaskLeaseRequest) (gitopsreconciliation.TaskLeaseResult, error) {
	if b.rbac == nil {
		return gitopsreconciliation.TaskLeaseResult{Granted: false, Reason: "rbac bridge provider unavailable", Directive: "DENY"}, nil
	}
	result, err := b.rbac.EvaluateTaskLease(ctx, rbac.TaskLeaseEvalRequest{
		TokenExpired:          req.TokenExpired,
		HasSignedTaskLease:    req.LeaseToken != "",
		LeaseExpired:          req.LeaseExpired,
		IsIdempotentBatchStep: false,
		RefreshSucceeded:      !req.TokenExpired,
	})
	return gitopsreconciliation.TaskLeaseResult{
		Granted:   err == nil && result.Granted,
		Reason:    result.Reason,
		Directive: result.Directive,
	}, err
}
