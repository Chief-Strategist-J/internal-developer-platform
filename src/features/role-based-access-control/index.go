// Package rbac is the public surface of the role-based-access-control feature (V.3 role 13, X-05).
//
// Blueprint:
//  1. Only RBACModule, BuildModule, provided authz.Authorizer capability, and declared bridge exposes are exported.
//  2. All internal packages (service, contracts, domain, infrastructure, api) sit under internal/.
//
// Invariants:
//   - No feature may import this package directly (G-02).
//   - Only src/app/ and src/main.go import this root package (L-01, X-05).
package rbac

import (
	"context"
	"time"
)

type TaskLeaseEvalRequest struct {
	TokenExpired          bool
	HasSignedTaskLease    bool
	LeaseExpired          bool
	IsIdempotentBatchStep bool
	RefreshSucceeded      bool
}

type TaskLeaseEvalResult struct {
	Granted       bool
	Reason        string
	Directive     string
	ViolationCode string
}

type AsyncJobDriftEvalResult struct {
	Granted       bool
	Reason        string
	Directive     string
	ViolationCode string
}

type RBACBridgeProvider interface {
	EvaluateTaskLease(ctx context.Context, req TaskLeaseEvalRequest) (TaskLeaseEvalResult, error)
	EvaluateAsyncJobDrift(ctx context.Context, subjectID string, jti string, queueLatency time.Duration) (AsyncJobDriftEvalResult, error)
}
