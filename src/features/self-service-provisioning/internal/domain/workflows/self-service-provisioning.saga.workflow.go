/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING SAGA WORKFLOW (V.3 role 09)

 1. Role:
    - Pure saga step & compensation blueprint for self-service provisioning (INV-02/06/07/08/10/12, EC-08).
 2. Invariants:
    - Pure definitions only; zero goroutines, zero IO, zero inline comments below this header.
*/
package workflows

import "internal-developer-platform/src/features/self-service-provisioning/internal/contracts"

func BuildProvisioningSagaSteps() []contracts.SagaStepSpec {
	return []contracts.SagaStepSpec{
		{StepID: "S1", Name: "reserve-capacity-and-name", IdempotencyMode: "atomic-db-tx", Compensation: "release-reservation"},
		{StepID: "S2", Name: "verify-approval-and-plan-pin", IdempotencyMode: "cas-consumed", Compensation: "none"},
		{StepID: "S3", Name: "scan-and-commit-gitops-manifests", IdempotencyMode: "commit-sha-pin", Compensation: "archive-repo", PastPointOfNoReturn: true},
		{StepID: "S4", Name: "reconcile-argocd-and-observe-ready", IdempotencyMode: "label-lookup-op-id", Compensation: "teardown-dag-dns-first", PastPointOfNoReturn: true},
	}
}

func ResolvePointOfNoReturnAction(pastPointOfNoReturn bool, forwardPossible bool) string {
	if pastPointOfNoReturn && forwardPossible {
		return "FORWARD_COMPLETE"
	}
	return "COMPENSATE_BY_CLASS"
}
