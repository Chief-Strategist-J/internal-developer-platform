/*
ALGORITHM & ARCHITECTURE BLUEPRINT: WORKLOAD <-> PLATFORM REGIME BRIDGE (X-03, GO-INT-001)

 1. Role:
    - Implements kubernetes-workload-provisioning's RegimeGuardBridge using platform-regime's
    RegimeBridgeProvider via root index imports only.
 2. Invariants:
    - Zero retry logic, business rules, or internal/ imports.
*/
package bridges

import (
	"context"

	kubernetesworkloadprovisioning "internal-developer-platform/src/features/kubernetes-workload-provisioning"
	platformregime "internal-developer-platform/src/features/platform-regime"
)

type WorkloadRegimeBridge struct {
	regime platformregime.RegimeBridgeProvider
}

func NewWorkloadRegimeBridge(regime platformregime.RegimeBridgeProvider) *WorkloadRegimeBridge {
	return &WorkloadRegimeBridge{regime: regime}
}

func (b *WorkloadRegimeBridge) EvaluateWorkloadRegime(ctx context.Context, req kubernetesworkloadprovisioning.RegimeEvaluationRequest) (kubernetesworkloadprovisioning.RegimeEvaluationResult, error) {
	if b.regime == nil {
		return kubernetesworkloadprovisioning.RegimeEvaluationResult{Allowed: true}, nil
	}
	ok, violations, err := b.regime.EvaluateWorkloadCompliance(ctx, req.TenantID, platformregime.WorkloadComplianceEvalInput{
		Namespace:           req.Namespace,
		AuditRetentionDays:  req.AuditRetentionDays,
		HasEncryptedSecrets: req.HasEncryptedSecrets,
		KMSKeyURI:           req.KMSKeyURI,
	})
	if err != nil {
		return kubernetesworkloadprovisioning.RegimeEvaluationResult{Allowed: false}, err
	}
	mapped := make([]kubernetesworkloadprovisioning.RegimeViolation, 0, len(violations))
	for _, v := range violations {
		mapped = append(mapped, kubernetesworkloadprovisioning.RegimeViolation{
			Rule:    v.Rule,
			Message: v.Message,
		})
	}
	return kubernetesworkloadprovisioning.RegimeEvaluationResult{
		Allowed:    ok,
		Violations: mapped,
	}, nil
}
