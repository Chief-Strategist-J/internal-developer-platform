/*
ALGORITHM & ARCHITECTURE BLUEPRINT: GITOPS RECONCILIATION INDEX (V.3 role 13, X-05)

 1. Public Surface:
    - Exports only GitOpsModule, BuildModule, and TaskLeaseBridge consumer contract.
    - Internal services, adapters, and ports stay behind internal/.
*/
package gitopsreconciliation

import "context"

type TaskLeaseRequest struct {
	SubjectID    string
	TenantID     string
	TaskType     string
	LeaseToken   string
	TokenExpired bool
	LeaseExpired bool
}

type TaskLeaseResult struct {
	Granted   bool
	Reason    string
	Directive string
}

type TaskLeaseBridge interface {
	EvaluateTaskLease(ctx context.Context, req TaskLeaseRequest) (TaskLeaseResult, error)
}
