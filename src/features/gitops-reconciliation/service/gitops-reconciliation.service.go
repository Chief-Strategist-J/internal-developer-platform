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

	for _, m := range manifests {
		if err := rules.ValidateTenantProjectGuardrails(true, m.Kind); err != nil {
			observability.RecordError(span, err)
			return nil, err
		}
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

	causeMsg := "deployment health check failed"
	if cause != nil {
		causeMsg = cause.Error()
	}

	record.RollbackDetails = &contracts.RollbackResult{
		PreviousRevision: record.TargetRevision,
		RestoredRevision: restoreRevision,
		Reason:           causeMsg,
		Success:          true,
	}

	restoredPhase, err := machines.TransitionSyncState(rollingBackPhase, machines.EventCompleteRollback)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	record.SyncPhase = restoredPhase
	record.HealthStatus = contracts.HealthStatusHealthy
	record.CurrentRevision = restoreRevision
	record.Version = record.Version + 1

	if err := s.store.Update(ctx, *record); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	return &contracts.SyncResult{
		Reconciliation: *record,
		AppliedHooks:   1,
		Message:        fmt.Sprintf("deployment failed; recorded rollback to %s", restoreRevision),
	}, nil
}

func (s *GitOpsReconciliationService) IngestWebhook(
	ctx context.Context,
	payload contracts.GitOpsWebhookPayload,
	signature string,
	webhookSecret string,
	payloadBytes []byte,
) (*contracts.GitOpsWebhookResult, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "gitops.reconciliation.webhook",
		attribute.String("repository.url", payload.RepositoryURL),
		attribute.String("branch", payload.Branch),
		attribute.String("commit.sha", payload.CommitSHA),
	)
	defer span.End()

	if err := contracts.ValidateWebhookPayload(payload); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	if webhookSecret != "" && !rules.VerifyWebhookHMAC(payloadBytes, signature, webhookSecret) {
		err := errors.NewUnauthorizedError("invalid webhook HMAC signature")
		observability.RecordError(span, err)
		return nil, err
	}

	refreshedApps, err := s.argoCD.RefreshAppsForRepo(ctx, payload.RepositoryURL, payload.Branch)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	return &contracts.GitOpsWebhookResult{
		Accepted:       true,
		RefreshedApps:  refreshedApps,
		TriggeredSyncs: make([]string, 0),
		Message:        fmt.Sprintf("webhook accepted: refreshed %d applications for branch %s", len(refreshedApps), payload.Branch),
	}, nil
}

func (s *GitOpsReconciliationService) InspectDrift(
	ctx context.Context,
	tenantID string,
	reconciliationID string,
) (*contracts.DetailedDriftReport, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "gitops.reconciliation.inspect_drift",
		attribute.String("tenant.id", tenantID),
		attribute.String("reconciliation.id", reconciliationID),
	)
	defer span.End()

	record, err := s.store.GetByID(ctx, tenantID, reconciliationID)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	if record == nil {
		err := errors.NewNotFoundError("reconciliation record not found")
		observability.RecordError(span, err)
		return nil, err
	}

	report, err := s.argoCD.GetDetailedDrift(ctx, record.AppName, record.TargetRevision)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	return report, nil
}

func (s *GitOpsReconciliationService) GetReconciliation(ctx context.Context, tenantID string, id string) (*contracts.ReconciliationRecord, error) {
	return s.store.GetByID(ctx, tenantID, id)
}

func (s *GitOpsReconciliationService) ListReconciliations(ctx context.Context, filter contracts.FilterCriteria) ([]contracts.ReconciliationRecord, error) {
	return s.store.List(ctx, filter)
}
