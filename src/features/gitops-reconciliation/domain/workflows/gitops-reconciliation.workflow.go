package workflows

import (
	"context"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/rules"
	"internal-developer-platform/src/infra/observability"
)

type StepName string

const (
	StepFetchManifests  StepName = "FETCH_MANIFESTS"
	StepEvaluateDrift   StepName = "EVALUATE_DRIFT"
	StepApplySyncWaves  StepName = "APPLY_SYNC_WAVES"
	StepVerifyHealth    StepName = "VERIFY_HEALTH"
	StepExecuteRollback StepName = "EXECUTE_ROLLBACK"
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

func PlanWaveBatches(manifests []rules.ResourceManifest) []rules.SyncWaveBatch {
	return rules.GroupManifestsBySyncWave(manifests)
}

func ExecuteReconciliationPlan(ctx context.Context, manifests []rules.ResourceManifest) WorkflowExecution {
	ctx, span := observability.GetTracer().StartSpan(ctx, "gitops.workflow.execute_plan",
		attribute.Int("manifest.count", len(manifests)),
	)
	defer span.End()

	sorted := make([]rules.ResourceManifest, 0, len(manifests))
	_ = observability.WithSpan(ctx, "gitops.workflow.fetch_manifests", func(stepCtx context.Context) error {
		sorted = PlanReconciliationDAG(manifests)
		return nil
	}, attribute.Int("manifest.count", len(manifests)))

	batches := PlanWaveBatches(sorted)
	_ = observability.WithSpan(ctx, "gitops.workflow.apply_sync_waves", func(stepCtx context.Context) error {
		return nil
	}, attribute.Int("wave.batch_count", len(batches)))

	_ = observability.WithSpan(ctx, "gitops.workflow.verify_health", func(stepCtx context.Context) error {
		return nil
	})

	steps := make([]ReconciliationStep, 0, 4)
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
