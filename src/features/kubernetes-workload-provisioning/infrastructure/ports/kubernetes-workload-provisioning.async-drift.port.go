// Package ports declares the AsyncJobDriftPort — the kubernetes-workload-provisioning feature's
// consumer port for the RBAC async-job-drift capability (X-03, V.3 role 10).
//
// Blueprint:
//  1. This port is owned by kubernetes-workload-provisioning, not by RBAC.
//  2. The bridge in src/app/bridges/workload__rbac.bridge.go implements it using RBAC.
//  3. kubernetes-workload-provisioning calls only this port; it never imports the rbac package.
//
// Invariants:
//   - Types here use workload-provisioning vocabulary, not RBAC vocabulary.
//   - Fail-closed: if RBAC is unavailable, the bridge returns an error and the job is treated as revoked.
package ports

import (
	"context"
	"time"
)

type AsyncJobDriftRequest struct {
	SubjectID    string
	JTI          string
	QueueLatency time.Duration
}

type AsyncJobDriftResult struct {
	Revoked   bool
	Reason    string
	Granted   bool
	Directive string
}

type AsyncJobDriftPort interface {
	EvaluateAsyncJobDrift(ctx context.Context, req AsyncJobDriftRequest) (AsyncJobDriftResult, error)
}
