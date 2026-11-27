/*
ALGORITHM & ARCHITECTURE BLUEPRINT: WORKLOAD <-> POLICY-AS-CODE BRIDGE (X-03, GO-INT-001)

 1. Role:
    - Implements kubernetes-workload-provisioning's PolicyExceptionBridge using policy-as-code's
    PolicyBridgeProvider via root index imports only.
 2. Invariants:
    - Zero retry logic, business rules, or internal/ imports.
*/
package bridges

import (
	"context"

	kubernetesworkloadprovisioning "internal-developer-platform/src/features/kubernetes-workload-provisioning"
	policyascode "internal-developer-platform/src/features/policy-as-code"
)

type WorkloadPolicyBridge struct {
	policy policyascode.PolicyBridgeProvider
}

func NewWorkloadPolicyBridge(policy policyascode.PolicyBridgeProvider) *WorkloadPolicyBridge {
	return &WorkloadPolicyBridge{policy: policy}
}

func (b *WorkloadPolicyBridge) GetActiveExceptionForWorkload(ctx context.Context, tenantID string, namespace string, workloadName string) (*kubernetesworkloadprovisioning.ActivePolicyException, error) {
	if b.policy == nil {
		return nil, nil
	}
	exc, err := b.policy.GetActiveExceptionForWorkload(ctx, tenantID, namespace, workloadName)
	if err != nil || exc == nil {
		return nil, err
	}
	return &kubernetesworkloadprovisioning.ActivePolicyException{
		PublicID:   exc.PublicID,
		ApproverID: exc.ApproverID,
	}, nil
}
