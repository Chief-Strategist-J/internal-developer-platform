package service

import (
	"context"
	"time"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/machines"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/workflows"
	"internal-developer-platform/src/features/gitops-reconciliation/infrastructure/ports"
	"internal-developer-platform/src/shared/errors"
)

type GitOpsReconciliationService struct {
	store  ports.GitOpsStorePort
	argoCD ports.ArgoCDClientPort
}

func NewGitOpsReconciliationService(store ports.GitOpsStorePort, argoCD ports.ArgoCDClientPort) *GitOpsReconciliationService {
	return &GitOpsReconciliationService{
		store:  store,
		argoCD: argoCD,
	}
}

func (s *GitOpsReconciliationService) TriggerReconciliation(ctx context.Context, cmd contracts.SyncCommand) (*contracts.SyncResult, error) {
	if err := contracts.ValidateSyncCommand(cmd); err != nil {
		return nil, err
	}

	existing, err := s.store.GetByIdempotencyKey(ctx, cmd.TenantID, cmd.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return &contracts.SyncResult{
			Reconciliation: *existing,
			AppliedHooks:   0,
			Message:        "idempotent replay",
		}, nil
	}

	manifests, _, err := s.argoCD.DiffManifests(ctx, cmd.AppName, cmd.TargetRevision)
	if err != nil {
		return nil, err
	}

	execution := workflows.ExecuteReconciliationPlan(ctx, manifests)
	now := time.Now().UTC()

	initialRecord := contracts.ReconciliationRecord{
		ID:              cmd.ID,
		TenantID:        cmd.TenantID,
		ServiceID:       cmd.ServiceID,
		AppName:         cmd.AppName,
		TargetRevision:  cmd.TargetRevision,
		CurrentRevision: "",
		SyncPhase:       contracts.SyncPhasePending,
		HealthStatus:    contracts.HealthStatusProgressing,
		IdempotencyKey:  cmd.IdempotencyKey,
		Version:         1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.store.Save(ctx, initialRecord); err != nil {
		return nil, err
	}

	inProgressPhase, err := machines.TransitionSyncState(initialRecord.SyncPhase, machines.EventStartSync)
	if err != nil {
		return nil, err
	}

	initialRecord.SyncPhase = inProgressPhase
	hooksApplied, err := s.argoCD.TriggerSync(ctx, cmd.AppName, cmd.TargetRevision, cmd.Prune, cmd.DryRun)
	if err != nil {
		failedPhase, _ := machines.TransitionSyncState(inProgressPhase, machines.EventFailSync)
		initialRecord.SyncPhase = failedPhase
		initialRecord.HealthStatus = contracts.HealthStatusDegraded
		initialRecord.Version = 2
		_ = s.store.Update(ctx, initialRecord)
		return nil, errors.NewConflictError("argocd sync failed: " + err.Error())
	}

	finalPhase, err := machines.TransitionSyncState(inProgressPhase, machines.EventCompleteSync)
	if err != nil {
		return nil, err
	}

	hasFailures := false
	for _, step := range execution.Steps {
		if !step.Passed {
			hasFailures = true
			break
		}
	}

	initialRecord.SyncPhase = finalPhase
	initialRecord.HealthStatus = workflows.DetermineFinalHealth(hasFailures)
	initialRecord.CurrentRevision = cmd.TargetRevision
	initialRecord.Version = 2

	if err := s.store.Update(ctx, initialRecord); err != nil {
		return nil, err
	}

	return &contracts.SyncResult{
		Reconciliation: initialRecord,
		AppliedHooks:   hooksApplied,
		Message:        "reconciliation completed successfully",
	}, nil
}

func (s *GitOpsReconciliationService) GetReconciliation(ctx context.Context, tenantID string, id string) (*contracts.ReconciliationRecord, error) {
	if tenantID == "" || id == "" {
		return nil, errors.NewValidationError("tenantId and id are required")
	}
	return s.store.GetByID(ctx, tenantID, id)
}

func (s *GitOpsReconciliationService) ListReconciliations(ctx context.Context, filter contracts.FilterCriteria) ([]contracts.ReconciliationRecord, error) {
	if filter.TenantID == "" {
		return nil, errors.NewValidationError("tenantId is required")
	}
	return s.store.List(ctx, filter)
}

