package service

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/machines"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/rules"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/workflows"
	"internal-developer-platform/src/features/gitops-reconciliation/infrastructure/ports"
	"internal-developer-platform/src/infra/observability"
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
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "gitops.reconciliation.trigger",
		attribute.String("tenant.id", cmd.TenantID),
		attribute.String("service.id", cmd.ServiceID),
		attribute.String("app.name", cmd.AppName),
		attribute.String("target.revision", cmd.TargetRevision),
	)
	defer span.End()

	if err := contracts.ValidateSyncCommand(cmd); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	existing, err := s.store.GetByIdempotencyKey(ctx, cmd.TenantID, cmd.IdempotencyKey)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	if existing != nil {
		span.SetAttributes(attribute.Bool("idempotent.replay", true))
		return &contracts.SyncResult{
			Reconciliation: *existing,
			AppliedHooks:   0,
			Message:        "idempotent replay",
		}, nil
	}

	lastGoodRev := cmd.LastKnownGoodRevision
	if lastGoodRev == "" {
		latestSynced, err := s.store.GetLatestSyncedRecord(ctx, cmd.TenantID, cmd.ServiceID)
		if err == nil && latestSynced != nil {
			lastGoodRev = latestSynced.CurrentRevision
		}
	}

	manifests, _, err := s.argoCD.DiffManifests(ctx, cmd.AppName, cmd.TargetRevision)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	execution := workflows.ExecuteReconciliationPlan(ctx, manifests)
	now := time.Now().UTC()

	initialRecord := contracts.ReconciliationRecord{
		ID:                    cmd.ID,
		TenantID:              cmd.TenantID,
		ServiceID:             cmd.ServiceID,
		AppName:               cmd.AppName,
		TargetRevision:        cmd.TargetRevision,
		CurrentRevision:       "",
		LastKnownGoodRevision: lastGoodRev,
		SyncPhase:             contracts.SyncPhasePending,
		HealthStatus:          contracts.HealthStatusProgressing,
		RollbackTriggered:     false,
		IdempotencyKey:        cmd.IdempotencyKey,
		Version:               1,
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	if err := s.store.Save(ctx, initialRecord); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	inProgressPhase, err := machines.TransitionSyncState(initialRecord.SyncPhase, machines.EventStartSync)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	initialRecord.SyncPhase = inProgressPhase
	hooksApplied, syncErr := s.argoCD.TriggerSync(ctx, cmd.AppName, cmd.TargetRevision, cmd.Prune, cmd.DryRun)

	var appHealth contracts.HealthStatus = contracts.HealthStatusHealthy
	if syncErr == nil {
		_, health, _, err := s.argoCD.GetApplicationStatus(ctx, cmd.AppName)
		if err == nil {
			appHealth = health
		}
	}

	if rules.ShouldTriggerRollback(syncErr, appHealth, lastGoodRev) {
		return s.executeStatefulRollback(ctx, &initialRecord, lastGoodRev, syncErr)
	}

	if syncErr != nil {
		failedPhase, _ := machines.TransitionSyncState(inProgressPhase, machines.EventFailSync)
		initialRecord.SyncPhase = failedPhase
		initialRecord.HealthStatus = contracts.HealthStatusDegraded
		initialRecord.Version = 2
		_ = s.store.Update(ctx, initialRecord)
		observability.RecordError(span, syncErr)
		return nil, errors.NewConflictError("argocd sync failed: " + syncErr.Error())
	}

	finalPhase, err := machines.TransitionSyncState(inProgressPhase, machines.EventCompleteSync)
	if err != nil {
		observability.RecordError(span, err)
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

	if err := s.store.Update(ctx, initialRecord); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	initialRecord.Version = initialRecord.Version + 1

	span.SetAttributes(
		attribute.String("sync.phase", string(finalPhase)),
		attribute.String("health.status", string(initialRecord.HealthStatus)),
		attribute.Int("applied.hooks", hooksApplied),
	)

	return &contracts.SyncResult{
		Reconciliation: initialRecord,
		AppliedHooks:   hooksApplied,
		Message:        "reconciliation completed successfully",
	}, nil
}

func (s *GitOpsReconciliationService) executeStatefulRollback(
	ctx context.Context,
	record *contracts.ReconciliationRecord,
	restoreRevision string,
	cause error,
) (*contracts.SyncResult, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "gitops.reconciliation.rollback",
		attribute.String("tenant.id", record.TenantID),
		attribute.String("service.id", record.ServiceID),
		attribute.String("failed.revision", record.TargetRevision),
		attribute.String("restored.revision", restoreRevision),
	)
	defer span.End()

	rollingBackPhase, err := machines.TransitionSyncState(record.SyncPhase, machines.EventTriggerRollback)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	record.SyncPhase = rollingBackPhase
	record.RollbackTriggered = true
	if err := s.store.Update(ctx, *record); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	record.Version = record.Version + 1

	reason := "health check degraded"
	if cause != nil {
		reason = cause.Error()
	}

	hooks, rollbackErr := s.argoCD.TriggerSync(ctx, record.AppName, restoreRevision, true, false)
	if rollbackErr != nil {
		failedPhase, _ := machines.TransitionSyncState(record.SyncPhase, machines.EventFailRollback)
		record.SyncPhase = failedPhase
		record.HealthStatus = contracts.HealthStatusDegraded
		record.RollbackDetails = &contracts.RollbackResult{
			PreviousRevision: record.TargetRevision,
			RestoredRevision: restoreRevision,
			Reason:           fmt.Sprintf("rollback failed: %s (original cause: %s)", rollbackErr.Error(), reason),
			Success:          false,
		}
		_ = s.store.Update(ctx, *record)
		record.Version = record.Version + 1
		observability.RecordError(span, rollbackErr)
		return nil, errors.NewConflictError("stateful rollback execution failed: " + rollbackErr.Error())
	}

	syncedPhase, err := machines.TransitionSyncState(record.SyncPhase, machines.EventCompleteRollback)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	record.SyncPhase = syncedPhase
	record.CurrentRevision = restoreRevision
	record.HealthStatus = contracts.HealthStatusHealthy
	record.RollbackDetails = &contracts.RollbackResult{
		PreviousRevision: record.TargetRevision,
		RestoredRevision: restoreRevision,
		Reason:           reason,
		Success:          true,
	}

	if err := s.store.Update(ctx, *record); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	record.Version = record.Version + 1

	span.SetAttributes(
		attribute.Bool("rollback.success", true),
		attribute.String("restored.revision", restoreRevision),
	)

	return &contracts.SyncResult{
		Reconciliation: *record,
		AppliedHooks:   hooks,
		Message:        fmt.Sprintf("stateful rollback succeeded to %s due to: %s", restoreRevision, reason),
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
