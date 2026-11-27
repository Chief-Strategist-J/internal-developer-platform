// Package ports declares the TaskLeaseGuardPort — the gitops-reconciliation feature's
// consumer port for the RBAC task-lease capability (X-03, V.3 role 10).
//
// Blueprint:
//  1. This port is owned by gitops-reconciliation, not by RBAC.
//  2. The bridge in src/app/bridges/gitops__rbac.bridge.go implements it using RBAC.
//  3. gitops-reconciliation calls only this port; it never imports the rbac package.
//
// Invariants:
//   - Types here use gitops-reconciliation vocabulary, not RBAC vocabulary.
//   - If RBAC is unavailable, the bridge returns an error and gitops fails-closed.
package ports

import (
	"context"
)

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

type TaskLeaseGuardPort interface {
	EvaluateTaskLease(ctx context.Context, req TaskLeaseRequest) (TaskLeaseResult, error)
}
