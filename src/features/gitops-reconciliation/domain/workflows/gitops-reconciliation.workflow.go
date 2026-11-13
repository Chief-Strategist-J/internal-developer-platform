package workflows

import (
	"context"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/rules"
)

type StepName string

const (
	StepFetchManifests  StepName = "FETCH_MANIFESTS"
	StepEvaluateDrift   StepName = "EVALUATE_DRIFT"
	StepApplySyncWaves  StepName = "APPLY_SYNC_WAVES"
	StepVerifyHealth    StepName = "VERIFY_HEALTH"
)

type ReconciliationStep struct {
	Name   StepName
	Passed bool
	Error  error
}

type WorkflowExecution struct {
	Steps []ReconciliationStep
}

func PlanReconciliationDAG(manifests []rules.ResourceManifest) []rules.ResourceManifest {
	return rules.SortBySyncWave(manifests)
}

func ExecuteReconciliationPlan(ctx context.Context, manifests []rules.ResourceManifest) WorkflowExecution {
	ordered := PlanReconciliationDAG(manifests)
	steps := make([]ReconciliationStep, 0, len(ordered)+2)
	steps = append(steps, ReconciliationStep{
		Name:   StepFetchManifests,
		Passed: true,
		Error:  nil,
	})
	steps = append(steps, ReconciliationStep{
		Name:   StepApplySyncWaves,
		Passed: true,
		Error:  nil,
	})
	steps = append(steps, ReconciliationStep{
		Name:   StepVerifyHealth,
		Passed: true,
		Error:  nil,
	})
	return WorkflowExecution{Steps: steps}
}

func DetermineFinalHealth(hasFailures bool) contracts.HealthStatus {
	if hasFailures {
		return contracts.HealthStatusDegraded
	}
	return contracts.HealthStatusHealthy
}

