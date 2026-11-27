/*
ALGORITHM & ARCHITECTURE BLUEPRINT: WORKLOAD GOVERNANCE CONSUMER PORTS (X-03)

 1. Role:
    - Consumer-owned ports for kubernetes-workload-provisioning to ask platform-regime compliance
    and policy-as-code active admission exceptions without cross-feature imports (G-02).
 2. Invariants:
    - Implemented exclusively in src/app/bridges/.
    - Zero imports of platform-regime or policy-as-code.
*/
package ports

import "context"

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

type RegimeGuardPort interface {
	EvaluateWorkloadRegime(ctx context.Context, req RegimeEvaluationRequest) (RegimeEvaluationResult, error)
}

type ActivePolicyException struct {
	PublicID   string
	ApproverID string
}

type PolicyExceptionGuardPort interface {
	GetActiveExceptionForWorkload(ctx context.Context, tenantID string, namespace string, workloadName string) (*ActivePolicyException, error)
}
