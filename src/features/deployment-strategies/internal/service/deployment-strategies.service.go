/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DEPLOYMENT STRATEGIES SERVICE (V.3 role 07)

1. Purpose:
   - Primary domain service implementing progressive delivery orchestration.
   - Enforces request validation, plan verification, idempotency, and workflow step execution.
2. Invariants:
   - Zero inline comments below header.
*/

package service

import (
	"context"
	"fmt"
	"time"

	"internal-developer-platform/src/features/deployment-strategies/internal/contracts"
	"internal-developer-platform/src/features/deployment-strategies/internal/domain/machines"
	"internal-developer-platform/src/features/deployment-strategies/internal/domain/workflows"
	"internal-developer-platform/src/features/deployment-strategies/internal/infrastructure/ports"
	"internal-developer-platform/src/shared/errors"
)

type DeploymentStrategiesService struct {
	repo     ports.RepositoryPort
	traffic  ports.TrafficPort
	workflow *workflows.CanaryRolloutWorkflow
}

func NewDeploymentStrategiesService(
	repo ports.RepositoryPort,
	traffic ports.TrafficPort,
	orchestrator ports.OrchestratorPort,
	analysis ports.AnalysisPort,
) *DeploymentStrategiesService {
	workflow := workflows.NewCanaryRolloutWorkflow(traffic, orchestrator, analysis, repo)
	return &DeploymentStrategiesService{
		repo:     repo,
		traffic:  traffic,
		workflow: workflow,
	}
}

func (s *DeploymentStrategiesService) CreatePlan(ctx context.Context, plan contracts.RolloutPlan) error {
	if err := contracts.ValidateRolloutPlan(plan); err != nil {
		return errors.NewValidationError(err.Error())
	}
	if plan.ID == "" {
		plan.ID = fmt.Sprintf("plan-%d", time.Now().UnixNano())
	}
	if plan.PublicID == "" {
		plan.PublicID = fmt.Sprintf("dplan_%026x", time.Now().UnixNano())
	}
	if plan.Version == 0 {
		plan.Version = 1
	}
	return s.repo.CreatePlan(ctx, plan)
}

func (s *DeploymentStrategiesService) GetPlan(ctx context.Context, tenantID, planID string) (*contracts.RolloutPlan, error) {
	return s.repo.GetPlan(ctx, tenantID, planID)
}

func (s *DeploymentStrategiesService) StartRollout(ctx context.Context, input contracts.StartRolloutInput) (*contracts.RolloutExecution, error) {
	if err := contracts.ValidateStartRolloutInput(input); err != nil {
		return nil, errors.NewValidationError(err.Error())
	}
	plan, err := s.repo.GetPlan(ctx, input.TenantID, input.PlanID)
	if err != nil {
		return nil, err
	}

	active, err := s.repo.GetActiveExecutionForWorkload(ctx, input.TenantID, input.WorkloadID)
	if err != nil {
		return nil, err
	}
	if active != nil {
		if active.PlanID == input.PlanID && active.TargetImage == input.TargetImage {
			return active, nil
		}
		return nil, errors.NewConflictError(fmt.Sprintf("rollout execution %s already active for workload %s", active.ID, input.WorkloadID))
	}

	execution := contracts.RolloutExecution{
		ID:               fmt.Sprintf("exec-%d", time.Now().UnixNano()),
		PublicID:         fmt.Sprintf("dexec_%026x", time.Now().UnixNano()),
		TenantID:         input.TenantID,
		PlanID:           plan.ID,
		WorkloadID:       input.WorkloadID,
		TargetImage:      input.TargetImage,
		Status:           contracts.RolloutStatusInitializing,
		CurrentStepIndex: 0,
		CurrentWeightPct: 0,
		CorrelationID:    input.CorrelationID,
		Version:          1,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}

	if err := s.repo.CreateExecution(ctx, execution); err != nil {
		return nil, err
	}

	_ = s.workflow.ExecuteStepProgression(ctx, input.TenantID, execution.ID)
	return s.repo.GetExecution(ctx, input.TenantID, execution.ID)
}

func (s *DeploymentStrategiesService) AdvanceRolloutStep(ctx context.Context, tenantID, executionID string) error {
	return s.workflow.ExecuteStepProgression(ctx, tenantID, executionID)
}

func (s *DeploymentStrategiesService) AbortRollout(ctx context.Context, input contracts.AbortRolloutInput) error {
	exec, err := s.repo.GetExecution(ctx, input.TenantID, input.ExecutionID)
	if err != nil {
		return err
	}
	if err := machines.ValidateTransition(exec.Status, contracts.RolloutStatusAborting); err != nil {
		return err
	}
	_ = s.traffic.SetTrafficWeight(ctx, input.TenantID, exec.WorkloadID, 0)
	return s.repo.UpdateExecutionStatus(ctx, input.TenantID, exec.ID, contracts.RolloutStatusAborted, exec.CurrentStepIndex, 0, input.Reason, exec.Version)
}

func (s *DeploymentStrategiesService) GetExecution(ctx context.Context, tenantID, executionID string) (*contracts.RolloutExecution, error) {
	return s.repo.GetExecution(ctx, tenantID, executionID)
}
