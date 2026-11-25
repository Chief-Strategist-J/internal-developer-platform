// Package service provides the pure domain orchestrator for GitOps Reconciliation.
//
// Architectural Blueprint:
// - S1 Command: TriggerReconciliation, IngestWebhook, ConfigureSecretMechanism
// - S2 Query: GetReconciliation, ListReconciliations, InspectDrift, GetSecretMechanismConfig, ValidateSecretManifest
// - Enforces OpenTelemetry end-to-end child tracing (RunTraced / WithSpan), tenant isolation, and Inversion of Control.
// - Depends strictly on abstract outbound ports (GitOpsStorePort, ArgoCDClientPort, SecretMechanismPort).
// - Zero inline comments inside method bodies; execution is traced and returns typed errors.
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

const (
	SpanPrefixGitOps = "gitops.reconciliation"
	MaxPageSize      = 100
	DefaultPageSize  = 50
	MaxCASRetries    = 3
)

type GitOpsReconciliationService struct {
	store      ports.GitOpsStorePort
	argoCD     ports.ArgoCDClientPort
	secretPort ports.SecretMechanismPort
	outboxPort ports.OutboxPort
}

func NewGitOpsReconciliationService(store ports.GitOpsStorePort, argoCD ports.ArgoCDClientPort) *GitOpsReconciliationService {
	return &GitOpsReconciliationService{
		store:  store,
		argoCD: argoCD,
	}
}

func (s *GitOpsReconciliationService) SetSecretAdapter(port ports.SecretMechanismPort) {
	s.secretPort = port
}

func (s *GitOpsReconciliationService) SetOutboxPort(port ports.OutboxPort) {
	s.outboxPort = port
}

func (s *GitOpsReconciliationService) TriggerReconciliation(ctx context.Context, cmd contracts.SyncCommand) (*contracts.SyncResult, error) {
	return observability.RunTraced(ctx, SpanPrefixGitOps+".trigger", func(ctx context.Context) (*contracts.SyncResult, error) {
		if err := contracts.ValidateSyncCommand(cmd); err != nil {
			return nil, err
		}

		if err := s.evaluateReconciliationPolicyGates(ctx, cmd); err != nil {
			return nil, err
		}

		if replay, err := s.checkIdempotencyReplay(ctx, cmd); replay != nil || err != nil {
			return replay, err
		}

		lastGoodRev := s.resolveLastGoodRevision(ctx, cmd)

		manifests, err := s.validateManifestsAndGuardrails(ctx, cmd)
		if err != nil {
			return nil, err
		}

		execution := workflows.ExecuteReconciliationPlan(ctx, manifests)
		initialRecord := buildInitialRecord(cmd, lastGoodRev)

		if err := s.store.Save(ctx, initialRecord); err != nil {
			return nil, err
		}

		inProgressPhase, err := s.advanceSyncStateMachine(ctx, initialRecord.SyncPhase, machines.EventStartSync)
		if err != nil {
			return nil, err
		}
		initialRecord.SyncPhase = inProgressPhase

		hooksApplied, appHealth, syncErr := s.dispatchArgoCDSync(ctx, cmd)

		if rules.ShouldTriggerRollback(syncErr, appHealth, lastGoodRev) {
			return s.executeStatefulRollback(ctx, &initialRecord, lastGoodRev, syncErr)
		}

		if syncErr != nil {
			return nil, s.handleFailedSync(ctx, &initialRecord, inProgressPhase, cmd.TargetRevision, syncErr)
		}

		return s.finalizeSuccessRecord(ctx, &initialRecord, inProgressPhase, cmd.TargetRevision, execution, hooksApplied)
	},
		attribute.String("tenant.id", cmd.TenantID),
		attribute.String("service.id", cmd.ServiceID),
		attribute.String("app.name", cmd.AppName),
		attribute.String("target.revision", cmd.TargetRevision),
	)
}

func (s *GitOpsReconciliationService) IngestWebhook(
	ctx context.Context,
	payload contracts.GitOpsWebhookPayload,
	signature string,
	webhookSecret string,
	payloadBytes []byte,
) (*contracts.GitOpsWebhookResult, error) {
	return observability.RunTraced(ctx, SpanPrefixGitOps+".webhook", func(ctx context.Context) (*contracts.GitOpsWebhookResult, error) {
		if err := contracts.ValidateWebhookPayload(payload); err != nil {
			return nil, err
		}

		if webhookSecret != "" && !rules.VerifyWebhookHMAC(payloadBytes, signature, webhookSecret) {
			return nil, errors.NewUnauthorizedError("invalid webhook HMAC signature")
		}

		refreshedApps, err := s.argoCD.RefreshAppsForRepo(ctx, payload.RepositoryURL, payload.Branch)
		if err != nil {
			return nil, err
		}

		return &contracts.GitOpsWebhookResult{
			Accepted:       true,
			RefreshedApps:  refreshedApps,
			TriggeredSyncs: make([]string, 0),
			Message:        fmt.Sprintf("webhook accepted: refreshed %d applications for branch %s", len(refreshedApps), payload.Branch),
		}, nil
	},
		attribute.String("repository.url", payload.RepositoryURL),
		attribute.String("branch", payload.Branch),
		attribute.String("commit.sha", payload.CommitSHA),
	)
}

func (s *GitOpsReconciliationService) InspectDrift(
	ctx context.Context,
	tenantID string,
	reconciliationID string,
) (*contracts.DetailedDriftReport, error) {
	return observability.RunTraced(ctx, SpanPrefixGitOps+".inspect_drift", func(ctx context.Context) (*contracts.DetailedDriftReport, error) {
		record, err := s.store.GetByID(ctx, tenantID, reconciliationID)
		if err != nil {
			return nil, err
		}
		if record == nil {
			return nil, errors.NewNotFoundError("reconciliation record not found")
		}

		return s.argoCD.GetDetailedDrift(ctx, record.AppName, record.TargetRevision)
	},
		attribute.String("tenant.id", tenantID),
		attribute.String("reconciliation.id", reconciliationID),
	)
}

func (s *GitOpsReconciliationService) ValidateSecretManifest(
	ctx context.Context,
	mechanism contracts.SecretMechanismType,
	hasPlaintextData bool,
	pluginInjectsValues bool,
) contracts.SecretSyncValidationResult {
	res, _ := observability.RunTraced(ctx, SpanPrefixGitOps+".validate_secret_manifest", func(ctx context.Context) (contracts.SecretSyncValidationResult, error) {
		return rules.EvaluateSecretInGitOpsPath(mechanism, hasPlaintextData, pluginInjectsValues), nil
	}, attribute.String("secret.mechanism", string(mechanism)))
	return res
}

func (s *GitOpsReconciliationService) ConfigureSecretMechanism(
	ctx context.Context,
	cfg contracts.SecretBackendConfig,
) (*contracts.SecretBackendConfig, error) {
	return observability.RunTraced(ctx, SpanPrefixGitOps+".configure_secret_mechanism", func(ctx context.Context) (*contracts.SecretBackendConfig, error) {
		if s.secretPort == nil {
			return nil, errors.NewInternalError("secret mechanism port not configured")
		}
		if err := rules.ValidateSecretMechanismConfig(cfg); err != nil {
			return nil, err
		}
		return s.secretPort.ConfigureSecretMechanism(ctx, cfg)
	},
		attribute.String("tenant.id", cfg.TenantID),
		attribute.String("secret.mechanism", string(cfg.Mechanism)),
	)
}

func (s *GitOpsReconciliationService) GetSecretMechanismConfig(
	ctx context.Context,
	tenantID string,
) (*contracts.SecretBackendConfig, error) {
	return observability.RunTraced(ctx, SpanPrefixGitOps+".get_secret_mechanism_config", func(ctx context.Context) (*contracts.SecretBackendConfig, error) {
		if s.secretPort == nil {
			return nil, errors.NewInternalError("secret mechanism port not configured")
		}
		return s.secretPort.GetSecretMechanismConfig(ctx, tenantID)
	}, attribute.String("tenant.id", tenantID))
}

func (s *GitOpsReconciliationService) GetReconciliation(ctx context.Context, tenantID string, id string) (*contracts.ReconciliationRecord, error) {
	return observability.RunTraced(ctx, SpanPrefixGitOps+".get", func(ctx context.Context) (*contracts.ReconciliationRecord, error) {
		if tenantID == "" || id == "" {
			return nil, errors.NewValidationError("tenantId and id are required")
		}
		return s.store.GetByID(ctx, tenantID, id)
	},
		attribute.String("tenant.id", tenantID),
		attribute.String("reconciliation.id", id),
	)
}

func (s *GitOpsReconciliationService) ListReconciliations(ctx context.Context, filter contracts.FilterCriteria) ([]contracts.ReconciliationRecord, error) {
	return observability.RunTraced(ctx, SpanPrefixGitOps+".list", func(ctx context.Context) ([]contracts.ReconciliationRecord, error) {
		if filter.TenantID == "" {
			return nil, errors.NewValidationError("tenantId is required")
		}
		scoped := applyFilterAllowlist(filter)
		return s.store.List(ctx, scoped)
	},
		attribute.String("tenant.id", filter.TenantID),
		attribute.String("service.id", filter.ServiceID),
	)
}

func (s *GitOpsReconciliationService) evaluateReconciliationPolicyGates(ctx context.Context, cmd contracts.SyncCommand) error {
	return observability.WithSpan(ctx, SpanPrefixGitOps+".policy_gates", func(ctx context.Context) error {
		if err := rules.EvaluateSourceTrust(cmd.ProjectEnforcesSig, cmd.CommitSigned, cmd.AllowedSigningKey, cmd.RoleReviewed); err != nil {
			return err
		}
		if err := rules.EvaluateSyncWindow(cmd.AllowWindowExists, cmd.InAllowWindow, cmd.InDenyWindow, cmd.IsManualSync, cmd.WindowAllowsManual); err != nil {
			return err
		}
		if !rules.ShouldRetryFailedSHA(cmd.TargetRevision, cmd.LastFailedSHA) {
			return errors.NewConflictError("sync refused: target revision matches last failed SHA — resolve the failure before retrying")
		}
		return nil
	}, attribute.String("app.name", cmd.AppName))
}

func (s *GitOpsReconciliationService) checkIdempotencyReplay(ctx context.Context, cmd contracts.SyncCommand) (*contracts.SyncResult, error) {
	existing, err := s.store.GetByIdempotencyKey(ctx, cmd.TenantID, cmd.IdempotencyKey)
	if err != nil || existing == nil {
		return nil, err
	}
	return &contracts.SyncResult{
		Reconciliation: *existing,
		AppliedHooks:   0,
		Message:        "idempotent replay",
	}, nil
}

func (s *GitOpsReconciliationService) resolveLastGoodRevision(ctx context.Context, cmd contracts.SyncCommand) string {
	if cmd.LastKnownGoodRevision != "" {
		return cmd.LastKnownGoodRevision
	}
	latestSynced, err := s.store.GetLatestSyncedRecord(ctx, cmd.TenantID, cmd.ServiceID)
	if err == nil && latestSynced != nil {
		return latestSynced.CurrentRevision
	}
	return ""
}

func (s *GitOpsReconciliationService) validateManifestsAndGuardrails(ctx context.Context, cmd contracts.SyncCommand) ([]rules.ResourceManifest, error) {
	return observability.RunTraced(ctx, SpanPrefixGitOps+".validate_manifests", func(ctx context.Context) ([]rules.ResourceManifest, error) {
		manifests, _, err := s.argoCD.DiffManifests(ctx, cmd.AppName, cmd.TargetRevision)
		if err != nil {
			return nil, err
		}

		for _, m := range manifests {
			if err := rules.ValidateTenantProjectGuardrails(true, m.Kind); err != nil {
				return nil, err
			}
			if ownerApp, exists := cmd.OwnerAppMap[m.Kind+"/"+m.Name]; exists {
				if err := rules.ValidateSharedResourceOwnership(ownerApp, cmd.AppName); err != nil {
					return nil, err
				}
			}
			if cmd.Prune {
				if err := rules.ValidatePruneSafety(m.Kind, m.Annotations); err != nil {
					return nil, err
				}
			}
		}
		return manifests, nil
	}, attribute.String("app.name", cmd.AppName), attribute.String("target.revision", cmd.TargetRevision))
}

func buildInitialRecord(cmd contracts.SyncCommand, lastGoodRev string) contracts.ReconciliationRecord {
	now := time.Now().UTC()
	return contracts.ReconciliationRecord{
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
}

func (s *GitOpsReconciliationService) advanceSyncStateMachine(ctx context.Context, current contracts.SyncPhase, event machines.TransitionEvent) (contracts.SyncPhase, error) {
	return observability.RunTraced(ctx, SpanPrefixGitOps+".state_transition", func(ctx context.Context) (contracts.SyncPhase, error) {
		return machines.TransitionSyncState(current, event)
	}, attribute.String("sync.phase.from", string(current)), attribute.String("sync.event", string(event)))
}

func (s *GitOpsReconciliationService) dispatchArgoCDSync(ctx context.Context, cmd contracts.SyncCommand) (int, contracts.HealthStatus, error) {
	var hooksApplied int
	var appHealth contracts.HealthStatus = contracts.HealthStatusHealthy
	var syncErr error

	_ = observability.WithSpan(ctx, SpanPrefixGitOps+".argocd_sync", func(ctx context.Context) error {
		hooksApplied, syncErr = s.argoCD.TriggerSync(ctx, cmd.AppName, cmd.TargetRevision, cmd.Prune, cmd.DryRun)
		if syncErr == nil {
			_, health, _, err := s.argoCD.GetApplicationStatus(ctx, cmd.AppName)
			if err == nil {
				appHealth = health
			}
		}
		return syncErr
	}, attribute.String("app.name", cmd.AppName), attribute.String("target.revision", cmd.TargetRevision))

	return hooksApplied, appHealth, syncErr
}

func (s *GitOpsReconciliationService) handleFailedSync(
	ctx context.Context,
	record *contracts.ReconciliationRecord,
	inProgressPhase contracts.SyncPhase,
	targetRevision string,
	syncErr error,
) error {
	failedPhase, _ := machines.TransitionSyncState(inProgressPhase, machines.EventFailSync)
	record.SyncPhase = failedPhase
	record.HealthStatus = contracts.HealthStatusDegraded
	record.LastFailedSHA = targetRevision
	_ = s.retryTransaction(ctx, "update_failed", func(txCtx context.Context) error {
		record.Version = record.Version + 1
		return s.store.Update(txCtx, *record)
	})
	s.emitOutboxEvent(ctx, contracts.OutboxEventReconciliationFailed, *record)
	return errors.NewConflictError("argocd sync failed: " + syncErr.Error())
}

func (s *GitOpsReconciliationService) finalizeSuccessRecord(
	ctx context.Context,
	record *contracts.ReconciliationRecord,
	inProgressPhase contracts.SyncPhase,
	targetRevision string,
	execution workflows.WorkflowExecution,
	hooksApplied int,
) (*contracts.SyncResult, error) {
	finalPhase, err := s.advanceSyncStateMachine(ctx, inProgressPhase, machines.EventCompleteSync)
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

	record.SyncPhase = finalPhase
	record.HealthStatus = workflows.DetermineFinalHealth(hasFailures)
	record.CurrentRevision = targetRevision

	if err := s.retryTransaction(ctx, "update_synced", func(txCtx context.Context) error {
		return s.store.Update(txCtx, *record)
	}); err != nil {
		return nil, err
	}
	record.Version = record.Version + 1
	s.emitOutboxEvent(ctx, contracts.OutboxEventReconciliationSucceeded, *record)

	return &contracts.SyncResult{
		Reconciliation: *record,
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
	return observability.RunTraced(ctx, SpanPrefixGitOps+".rollback", func(ctx context.Context) (*contracts.SyncResult, error) {
		rollingBackPhase, err := s.transitionToRollingBack(ctx, record)
		if err != nil {
			return nil, err
		}
		return s.completeRollbackRecord(ctx, record, rollingBackPhase, restoreRevision, cause)
	},
		attribute.String("tenant.id", record.TenantID),
		attribute.String("service.id", record.ServiceID),
		attribute.String("failed.revision", record.TargetRevision),
		attribute.String("restored.revision", restoreRevision),
	)
}

func (s *GitOpsReconciliationService) transitionToRollingBack(ctx context.Context, record *contracts.ReconciliationRecord) (contracts.SyncPhase, error) {
	rollingBackPhase, err := machines.TransitionSyncState(record.SyncPhase, machines.EventTriggerRollback)
	if err != nil {
		return "", err
	}
	record.SyncPhase = rollingBackPhase
	record.RollbackTriggered = true
	if err := s.retryTransaction(ctx, "update_rolling_back", func(txCtx context.Context) error {
		return s.store.Update(txCtx, *record)
	}); err != nil {
		return "", err
	}
	s.emitOutboxEvent(ctx, contracts.OutboxEventRollbackTriggered, *record)
	return rollingBackPhase, nil
}

func (s *GitOpsReconciliationService) completeRollbackRecord(
	ctx context.Context,
	record *contracts.ReconciliationRecord,
	rollingBackPhase contracts.SyncPhase,
	restoreRevision string,
	cause error,
) (*contracts.SyncResult, error) {
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
		return nil, err
	}

	record.SyncPhase = restoredPhase
	record.HealthStatus = contracts.HealthStatusHealthy
	record.CurrentRevision = restoreRevision
	record.Version = record.Version + 1

	if err := s.retryTransaction(ctx, "update_rollback_complete", func(txCtx context.Context) error {
		return s.store.Update(txCtx, *record)
	}); err != nil {
		return nil, err
	}

	return &contracts.SyncResult{
		Reconciliation: *record,
		AppliedHooks:   1,
		Message:        fmt.Sprintf("deployment failed; recorded rollback to %s", restoreRevision),
	}, nil
}

func (s *GitOpsReconciliationService) retryTransaction(ctx context.Context, opName string, fn func(ctx context.Context) error) error {
	return observability.WithSpan(ctx, SpanPrefixGitOps+".retry_tx."+opName, func(txCtx context.Context) error {
		var lastErr error
		for attempt := 1; attempt <= MaxCASRetries; attempt++ {
			lastErr = fn(txCtx)
			if lastErr == nil {
				return nil
			}
			if !errors.IsConflictError(lastErr) {
				return lastErr
			}
		}
		return lastErr
	}, attribute.String("tx.operation", opName))
}

func (s *GitOpsReconciliationService) emitOutboxEvent(ctx context.Context, eventType contracts.OutboxEventType, record contracts.ReconciliationRecord) {
	if s.outboxPort == nil {
		return
	}
	_ = observability.WithSpan(ctx, SpanPrefixGitOps+".outbox_emit", func(outboxCtx context.Context) error {
		return s.outboxPort.Enqueue(outboxCtx, contracts.OutboxEvent{
			ID:          fmt.Sprintf("evt-%s-%d", record.ID, time.Now().UnixNano()),
			TenantID:    record.TenantID,
			AggregateID: record.ID,
			EventType:   eventType,
			AppName:     record.AppName,
			Revision:    record.CurrentRevision,
			OccurredAt:  time.Now().UTC(),
		})
	}, attribute.String("outbox.event_type", string(eventType)), attribute.String("aggregate.id", record.ID))
}

func applyFilterAllowlist(filter contracts.FilterCriteria) contracts.FilterCriteria {
	if filter.Limit <= 0 {
		filter.Limit = DefaultPageSize
	} else if filter.Limit > MaxPageSize {
		filter.Limit = MaxPageSize
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return filter
}
