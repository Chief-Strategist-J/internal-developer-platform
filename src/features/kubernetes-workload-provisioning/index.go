/*
ALGORITHM & ARCHITECTURE BLUEPRINT: KUBERNETES WORKLOAD PROVISIONING INDEX (V.3 role 13, X-05)

 1. Public Surface:
    - Exports only WorkloadModule, BuildModule, and consumer bridge port interfaces.
    - Internal services, adapters, and ports stay behind internal/.
*/
package kubernetesworkloadprovisioning

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

type AsyncDriftBridge interface {
	EvaluateAsyncJobDrift(ctx context.Context, req AsyncJobDriftRequest) (AsyncJobDriftResult, error)
}

type RegimeEvaluationRequest struct {
	TenantID            string
	Namespace           string
	AuditRetentionDays  int
	HasEncryptedSecrets bool
	KMSKeyURI           string
}

type RegimeViolation struct {
	Rule    string
	Message string
}

type RegimeEvaluationResult struct {
	Allowed    bool
	Violations []RegimeViolation
}

type RegimeGuardBridge interface {
	EvaluateWorkloadRegime(ctx context.Context, req RegimeEvaluationRequest) (RegimeEvaluationResult, error)
}

type ActivePolicyException struct {
	PublicID   string
	ApproverID string
}

type PolicyExceptionBridge interface {
	GetActiveExceptionForWorkload(ctx context.Context, tenantID string, namespace string, workloadName string) (*ActivePolicyException, error)
}
