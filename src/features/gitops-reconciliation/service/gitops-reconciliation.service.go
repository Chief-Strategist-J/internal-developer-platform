package service

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/machines"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/rules"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/workflows"
	"internal-developer-platform/src/features/gitops-reconciliation/infrastructure/ports"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type GitOpsReconciliationService struct {
	store      ports.GitOpsStorePort
	argoCD     ports.ArgoCDClientPort
	secretPort ports.SecretMechanismPort
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
		recordServiceFailure(ctx, span, err, "sync command validation failed", true, observability.String("app_name", cmd.AppName))
		return nil, err
	}

	if err := rules.EvaluateSourceTrust(cmd.ProjectEnforcesSig, cmd.CommitSigned, cmd.AllowedSigningKey, cmd.RoleReviewed); err != nil {
		recordServiceFailure(ctx, span, err, "source trust gate rejected sync", true, observability.String("app_name", cmd.AppName))
		return nil, err
	}

	if err := rules.EvaluateSyncWindow(cmd.AllowWindowExists, cmd.InAllowWindow, cmd.InDenyWindow, cmd.IsManualSync, cmd.WindowAllowsManual); err != nil {
		recordServiceFailure(ctx, span, err, "sync window gate rejected sync", true, observability.String("app_name", cmd.AppName))
		return nil, err
	}

	if !rules.ShouldRetryFailedSHA(cmd.TargetRevision, cmd.LastFailedSHA) {
		err := errors.NewConflictError("sync refused: target revision matches last failed SHA — resolve the failure before retrying")
		recordServiceFailure(ctx, span, err, "SHA retry dedup rejected sync", true, observability.String("target_revision", cmd.TargetRevision))
		return nil, err
	}

	existing, err := s.store.GetByIdempotencyKey(ctx, cmd.TenantID, cmd.IdempotencyKey)
	if err != nil {
		recordServiceFailure(ctx, span, err, "idempotency lookup failed", false, observability.String("idempotency_key", cmd.IdempotencyKey))
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
		recordServiceFailure(ctx, span, err, "argocd diff manifests failed", false, observability.String("app_name", cmd.AppName))
		return nil, err
	}

	for _, m := range manifests {
		if err := rules.ValidateTenantProjectGuardrails(true, m.Kind); err != nil {
			recordServiceFailure(ctx, span, err, "tenant project guardrail violation", true, observability.String("resource_kind", m.Kind))
			return nil, err
		}
		if ownerApp, exists := cmd.OwnerAppMap[m.Kind+"/"+m.Name]; exists {
			if err := rules.ValidateSharedResourceOwnership(ownerApp, cmd.AppName); err != nil {
				recordServiceFailure(ctx, span, err, "shared resource ownership conflict", true, observability.String("resource_kind", m.Kind), observability.String("owner_app", ownerApp))
				return nil, err
			}
		}
		if cmd.Prune {
			if err := rules.ValidatePruneSafety(m.Kind, m.Annotations); err != nil {
				recordServiceFailure(ctx, span, err, "prune safety check failed", true, observability.String("resource_kind", m.Kind))
				return nil, err
			}
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
		recordServiceFailure(ctx, span, err, "failed to persist initial reconciliation record", true, observability.String("reconciliation_id", initialRecord.ID))
		return nil, err
	}

	inProgressPhase, err := machines.TransitionSyncState(initialRecord.SyncPhase, machines.EventStartSync)
	if err != nil {
		recordServiceFailure(ctx, span, err, "invalid sync state transition to syncing", true, observability.String("phase", string(initialRecord.SyncPhase)))
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
		initialRecord.LastFailedSHA = cmd.TargetRevision
		initialRecord.Version = 2
		_ = s.store.Update(ctx, initialRecord)
		recordServiceFailure(ctx, span, syncErr, "argocd sync failed", false, observability.String("app_name", cmd.AppName))
		return nil, errors.NewConflictError("argocd sync failed: " + syncErr.Error())
	}

	finalPhase, err := machines.TransitionSyncState(inProgressPhase, machines.EventCompleteSync)
	if err != nil {
		recordServiceFailure(ctx, span, err, "invalid sync state transition to synced", true, observability.String("phase", string(inProgressPhase)))
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
		recordServiceFailure(ctx, span, err, "failed to update reconciliation record", true, observability.String("reconciliation_id", initialRecord.ID))
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
		recordServiceFailure(ctx, span, err, "webhook payload validation failed", true, observability.String("repository_url", payload.RepositoryURL))
		return nil, err
	}

	if webhookSecret != "" && !rules.VerifyWebhookHMAC(payloadBytes, signature, webhookSecret) {
		err := errors.NewUnauthorizedError("invalid webhook HMAC signature")
		recordServiceFailure(ctx, span, err, "webhook HMAC verification failed", true, observability.String("repository_url", payload.RepositoryURL))
		return nil, err
	}

	refreshedApps, err := s.argoCD.RefreshAppsForRepo(ctx, payload.RepositoryURL, payload.Branch)
	if err != nil {
		recordServiceFailure(ctx, span, err, "argocd refresh apps failed", false, observability.String("repository_url", payload.RepositoryURL))
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
		recordServiceFailure(ctx, span, err, "reconciliation lookup failed for drift inspection", true, observability.String("reconciliation_id", reconciliationID))
		return nil, err
	}
	if record == nil {
		err := errors.NewNotFoundError("reconciliation record not found")
		recordServiceFailure(ctx, span, err, "reconciliation record not found for drift inspection", true, observability.String("reconciliation_id", reconciliationID))
		return nil, err
	}

	report, err := s.argoCD.GetDetailedDrift(ctx, record.AppName, record.TargetRevision)
	if err != nil {
		recordServiceFailure(ctx, span, err, "argocd detailed drift inspection failed", false, observability.String("app_name", record.AppName))
		return nil, err
	}
	return report, nil
}

func (s *GitOpsReconciliationService) ValidateSecretManifest(
	ctx context.Context,
	mechanism contracts.SecretMechanismType,
	hasPlaintextData bool,
	pluginInjectsValues bool,
) contracts.SecretSyncValidationResult {
	tracer := observability.GetTracer()
	_, span := tracer.StartSpan(ctx, "gitops.reconciliation.validate_secret_manifest",
		attribute.String("secret.mechanism", string(mechanism)),
	)
	defer span.End()

	return rules.EvaluateSecretInGitOpsPath(mechanism, hasPlaintextData, pluginInjectsValues)
}

func (s *GitOpsReconciliationService) SetSecretAdapter(port ports.SecretMechanismPort) {
	s.secretPort = port
}

func recordServiceFailure(ctx context.Context, span trace.Span, err error, msg string, isClientErr bool, fields ...observability.Field) {
	observability.RecordError(span, err)
	logFields := append([]observability.Field{observability.String("error", err.Error())}, fields...)
	if isClientErr {
		observability.GetLogger().Warn(ctx, msg, logFields...)
		return
	}
	observability.GetLogger().Error(ctx, msg, logFields...)
}

func (s *GitOpsReconciliationService) ConfigureSecretMechanism(
	ctx context.Context,
	cfg contracts.SecretBackendConfig,
) (*contracts.SecretBackendConfig, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "gitops.reconciliation.configure_secret_mechanism",
		attribute.String("tenant.id", cfg.TenantID),
		attribute.String("secret.mechanism", string(cfg.Mechanism)),
	)
	defer span.End()

	if s.secretPort == nil {
		err := errors.NewInternalError("secret mechanism port not configured")
		recordServiceFailure(ctx, span, err, "secret mechanism port not configured", false)
		return nil, err
	}
	if err := rules.ValidateSecretMechanismConfig(cfg); err != nil {
		recordServiceFailure(ctx, span, err, "secret mechanism config domain validation failed", true, observability.String("mechanism", string(cfg.Mechanism)))
		return nil, err
	}
	saved, err := s.secretPort.ConfigureSecretMechanism(ctx, cfg)
	if err != nil {
		recordServiceFailure(ctx, span, err, "configure secret mechanism failed", true, observability.String("mechanism", string(cfg.Mechanism)))
		return nil, err
	}
	return saved, nil
}

func (s *GitOpsReconciliationService) GetSecretMechanismConfig(
	ctx context.Context,
	tenantID string,
) (*contracts.SecretBackendConfig, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "gitops.reconciliation.get_secret_mechanism_config",
		attribute.String("tenant.id", tenantID),
	)
	defer span.End()

	if s.secretPort == nil {
		err := errors.NewInternalError("secret mechanism port not configured")
		recordServiceFailure(ctx, span, err, "secret mechanism port not configured", false)
		return nil, err
	}
	cfg, err := s.secretPort.GetSecretMechanismConfig(ctx, tenantID)
	if err != nil {
		recordServiceFailure(ctx, span, err, "get secret mechanism config failed", true, observability.String("tenant_id", tenantID))
		return nil, err
	}
	return cfg, nil
}

func (s *GitOpsReconciliationService) GetReconciliation(ctx context.Context, tenantID string, id string) (*contracts.ReconciliationRecord, error) {
	ctx, span := observability.GetTracer().StartSpan(ctx, "gitops.reconciliation.get",
		attribute.String("tenant.id", tenantID),
		attribute.String("reconciliation.id", id),
	)
	defer span.End()

	rec, err := s.store.GetByID(ctx, tenantID, id)
	if err != nil {
		recordServiceFailure(ctx, span, err, "get reconciliation failed", true, observability.String("reconciliation_id", id))
		return nil, err
	}
	return rec, nil
}

func (s *GitOpsReconciliationService) ListReconciliations(ctx context.Context, filter contracts.FilterCriteria) ([]contracts.ReconciliationRecord, error) {
	ctx, span := observability.GetTracer().StartSpan(ctx, "gitops.reconciliation.list",
		attribute.String("tenant.id", filter.TenantID),
		attribute.String("service.id", filter.ServiceID),
	)
	defer span.End()

	records, err := s.store.List(ctx, filter)
	if err != nil {
		recordServiceFailure(ctx, span, err, "list reconciliations failed", false, observability.String("tenant_id", filter.TenantID))
		return nil, err
	}
	span.SetAttributes(attribute.Int("reconciliation.count", len(records)))
	return records, nil
}
