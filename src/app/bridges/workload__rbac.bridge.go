// Package bridges: workload__rbac.bridge wires the kubernetes-workload-provisioning feature's
// AsyncJobDriftPort to the RBAC feature's EvaluateAsyncJobDrift capability (X-03).
//
// Blueprint:
//  1. This bridge is the ONLY cross-feature wiring point for async job drift checks.
//  2. kubernetes-workload-provisioning calls its own AsyncJobDriftPort — never imports rbac directly.
//  3. This bridge adapts the consumer need-shaped port to the provider's exposed function.
//
// Invariants:
//   - Fail-closed: RBAC unavailability causes the job to be treated as revoked.
//   - No retry logic here; retries belong in adapters (R-01).
package bridges

import (
	"context"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/ports"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
)

type WorkloadRBACBridge struct {
	rbac *rbacService.RBACService
}

func NewWorkloadRBACBridge(rbac *rbacService.RBACService) *WorkloadRBACBridge {
	return &WorkloadRBACBridge{rbac: rbac}
}

func (b *WorkloadRBACBridge) EvaluateAsyncJobDrift(ctx context.Context, req ports.AsyncJobDriftRequest) (ports.AsyncJobDriftResult, error) {
	result, err := b.rbac.EvaluateAsyncJobDrift(
		ctx,
		req.SubjectID,
		req.JTI,
		req.QueueLatency,
		nil,
	)
	return ports.AsyncJobDriftResult{
		Revoked:   err != nil || !result.Granted,
		Granted:   err == nil && result.Granted,
		Reason:    result.Reason,
		Directive: result.Directive,
	}, err
}
