/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PROGRESSIVE CANARY ROLLOUT WORKFLOW (V.3 role 04, P0-01..P0-08)

1. Purpose:
   - Coordinates the progressive delivery lifecycle across ports (Traffic, Orchestrator, Analysis, Repository).
   - Enforces Invariant P0-03 (readiness before traffic shift).
   - Enforces Invariant P0-04 (automated abort on metric breach).
2. Invariants:
   - Zero inline comments below header.
*/

package workflows

import (
	"context"
	"fmt"
	"time"

	"internal-developer-platform/src/features/deployment-strategies/internal/contracts"
	"internal-developer-platform/src/features/deployment-strategies/internal/domain/machines"
	"internal-developer-platform/src/features/deployment-strategies/internal/domain/rules"
	"internal-developer-platform/src/features/deployment-strategies/internal/infrastructure/ports"
	"internal-developer-platform/src/shared/errors"
)

type CanaryRolloutWorkflow struct {
	traffic      ports.TrafficPort
	orchestrator ports.OrchestratorPort
	analysis     ports.AnalysisPort
	repo         ports.RepositoryPort
	drGuard      ports.DRGuardPort
}

func NewCanaryRolloutWorkflow(
	traffic ports.TrafficPort,
	orchestrator ports.OrchestratorPort,
	analysis ports.AnalysisPort,
	repo ports.RepositoryPort,
	drGuard ports.DRGuardPort,
) *CanaryRolloutWorkflow {
	return &CanaryRolloutWorkflow{
		traffic:      traffic,
		orchestrator: orchestrator,
		analysis:     analysis,
		repo:         repo,
		drGuard:      drGuard,
	}
}

func (w *CanaryRolloutWorkflow) ExecuteStepProgression(ctx context.Context, tenantID, executionID string) error {
	exec, err := w.repo.GetExecution(ctx, tenantID, executionID)
	if err != nil {
		return err
	}
	plan, err := w.repo.GetPlan(ctx, tenantID, exec.PlanID)
	if err != nil {
		return err
	}

	if w.drGuard != nil {
		fenced, fenceErr := w.drGuard.IsWorkloadFenced(ctx, tenantID, exec.WorkloadID)
		if fenceErr != nil {
			return fenceErr
		}
		if fenced {
			return w.abortRollout(ctx, tenantID, exec, fmt.Sprintf("disaster recovery fencing active for workload %s", exec.WorkloadID))
		}
	}

	if exec.CurrentStepIndex >= len(plan.Steps) {
		return w.promoteRollout(ctx, tenantID, exec)
	}
	currentStep := plan.Steps[exec.CurrentStepIndex]

	if err := machines.ValidateTransition(exec.Status, contracts.RolloutStatusRoutingStep); err != nil {
		return err
	}
	if err := w.repo.UpdateExecutionStatus(ctx, tenantID, exec.ID, contracts.RolloutStatusRoutingStep, exec.CurrentStepIndex, exec.CurrentWeightPct, "", exec.Version); err != nil {
		return err
	}
	exec.Status = contracts.RolloutStatusRoutingStep
	exec.Version++

	health, err := w.orchestrator.GetCanaryHealth(ctx, tenantID, exec.WorkloadID)
	if err != nil {
		return w.abortRollout(ctx, tenantID, exec, fmt.Sprintf("failed to get pod health: %v", err))
	}
	if err := rules.ValidateCanaryReadiness(health); err != nil {
		return w.abortRollout(ctx, tenantID, exec, err.Error())
	}

	if err := w.traffic.SetTrafficWeight(ctx, tenantID, exec.WorkloadID, currentStep.WeightPct); err != nil {
		return w.abortRollout(ctx, tenantID, exec, fmt.Sprintf("failed to shift traffic weight: %v", err))
	}

	if err := machines.ValidateTransition(exec.Status, contracts.RolloutStatusAnalyzing); err != nil {
		return err
	}
	if err := w.repo.UpdateExecutionStatus(ctx, tenantID, exec.ID, contracts.RolloutStatusAnalyzing, exec.CurrentStepIndex, currentStep.WeightPct, "", exec.Version); err != nil {
		return err
	}
	exec.Status = contracts.RolloutStatusAnalyzing
	exec.CurrentWeightPct = currentStep.WeightPct
	exec.Version++

	metricQuery := "sum(rate(http_requests_total{status=~'5..'}[1m])) / sum(rate(http_requests_total[1m])) * 100"
	samples, err := w.analysis.QueryMetricSamples(ctx, tenantID, exec.WorkloadID, metricQuery, 1*time.Minute)
	if err != nil {
		return w.abortRollout(ctx, tenantID, exec, fmt.Sprintf("metric query failed: %v", err))
	}

	verdict, measuredVal, evalErr := rules.EvaluateMetricThreshold(samples, plan.FailureThreshold, 1)
	if evalErr != nil || verdict == contracts.AnalysisVerdictFail {
		reason := fmt.Sprintf("metric analysis failure: measured %.2f%% exceeds threshold %.2f%%", measuredVal, plan.FailureThreshold)
		_ = w.repo.RecordAnalysisRun(ctx, contracts.RolloutAnalysisRun{
			ID:             fmt.Sprintf("danly_%d", time.Now().UnixNano()),
			PublicID:       fmt.Sprintf("danly_%026x", time.Now().UnixNano()),
			TenantID:       tenantID,
			ExecutionID:    exec.ID,
			MetricName:     "error_rate",
			Query:          metricQuery,
			MeasuredValue:  measuredVal,
			ThresholdValue: plan.FailureThreshold,
			Verdict:        contracts.AnalysisVerdictFail,
			CorrelationID:  exec.CorrelationID,
			CreatedAt:      time.Now().UTC(),
		})
		return w.abortRollout(ctx, tenantID, exec, reason)
	}

	_ = w.repo.RecordAnalysisRun(ctx, contracts.RolloutAnalysisRun{
		ID:             fmt.Sprintf("danly_%d", time.Now().UnixNano()),
		PublicID:       fmt.Sprintf("danly_%026x", time.Now().UnixNano()),
		TenantID:       tenantID,
		ExecutionID:    exec.ID,
		MetricName:     "error_rate",
		Query:          metricQuery,
		MeasuredValue:  measuredVal,
		ThresholdValue: plan.FailureThreshold,
		Verdict:        contracts.AnalysisVerdictPass,
		CorrelationID:  exec.CorrelationID,
		CreatedAt:      time.Now().UTC(),
	})

	nextIndex := exec.CurrentStepIndex + 1
	if nextIndex >= len(plan.Steps) {
		return w.promoteRollout(ctx, tenantID, exec)
	}

	return w.repo.UpdateExecutionStatus(ctx, tenantID, exec.ID, contracts.RolloutStatusRoutingStep, nextIndex, currentStep.WeightPct, "", exec.Version)
}

func (w *CanaryRolloutWorkflow) abortRollout(ctx context.Context, tenantID string, exec *contracts.RolloutExecution, reason string) error {
	_ = w.traffic.SetTrafficWeight(ctx, tenantID, exec.WorkloadID, 0)
	_ = w.orchestrator.ScaleCanaryReplicas(ctx, tenantID, exec.WorkloadID, 0)
	_ = w.repo.UpdateExecutionStatus(ctx, tenantID, exec.ID, contracts.RolloutStatusAborted, exec.CurrentStepIndex, 0, reason, exec.Version)
	return errors.NewInternalError(fmt.Sprintf("rollout aborted: %s", reason))
}

func (w *CanaryRolloutWorkflow) promoteRollout(ctx context.Context, tenantID string, exec *contracts.RolloutExecution) error {
	if err := machines.ValidateTransition(exec.Status, contracts.RolloutStatusPromoting); err != nil {
		return err
	}
	if err := w.repo.UpdateExecutionStatus(ctx, tenantID, exec.ID, contracts.RolloutStatusPromoting, exec.CurrentStepIndex, 100, "", exec.Version); err != nil {
		return err
	}
	exec.Status = contracts.RolloutStatusPromoting
	exec.Version++

	if err := w.orchestrator.PromoteStableRevision(ctx, tenantID, exec.WorkloadID, exec.TargetImage); err != nil {
		return w.abortRollout(ctx, tenantID, exec, fmt.Sprintf("promotion failed: %v", err))
	}

	_ = w.traffic.SetTrafficWeight(ctx, tenantID, exec.WorkloadID, 0)
	_ = w.orchestrator.ScaleCanaryReplicas(ctx, tenantID, exec.WorkloadID, 0)

	if err := machines.ValidateTransition(exec.Status, contracts.RolloutStatusCompleted); err != nil {
		return err
	}
	return w.repo.UpdateExecutionStatus(ctx, tenantID, exec.ID, contracts.RolloutStatusCompleted, exec.CurrentStepIndex, 100, "", exec.Version)
}
