// Package bridges: workload__rbac.bridge wires the kubernetes-workload-provisioning feature's
// AsyncDriftBridge to the RBAC feature's EvaluateAsyncJobDrift capability via index.go only (X-03, GO-INT-001).
//
// Blueprint:
//  1. Fail-closed: RBAC unavailability causes the job to be treated as revoked.
//  2. Zero internal/ imports — only root feature index packages are imported.
package bridges

import (
	"context"

	kubernetesworkloadprovisioning "internal-developer-platform/src/features/kubernetes-workload-provisioning"
	rbac "internal-developer-platform/src/features/role-based-access-control"
)

type WorkloadRBACBridge struct {
	rbac rbac.RBACBridgeProvider
}

func NewWorkloadRBACBridge(rbacProvider rbac.RBACBridgeProvider) *WorkloadRBACBridge {
	return &WorkloadRBACBridge{rbac: rbacProvider}
}

func (b *WorkloadRBACBridge) EvaluateAsyncJobDrift(ctx context.Context, req kubernetesworkloadprovisioning.AsyncJobDriftRequest) (kubernetesworkloadprovisioning.AsyncJobDriftResult, error) {
	if b.rbac == nil {
		return kubernetesworkloadprovisioning.AsyncJobDriftResult{Revoked: true, Granted: false, Reason: "rbac bridge provider unavailable", Directive: "DISCARD"}, nil
	}
	result, err := b.rbac.EvaluateAsyncJobDrift(ctx, req.SubjectID, req.JTI, req.QueueLatency)
	return kubernetesworkloadprovisioning.AsyncJobDriftResult{
		Revoked:   err != nil || !result.Granted,
		Granted:   err == nil && result.Granted,
		Reason:    result.Reason,
		Directive: result.Directive,
	}, err
}
