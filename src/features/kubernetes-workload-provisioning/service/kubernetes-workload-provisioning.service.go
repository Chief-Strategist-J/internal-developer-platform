// Package service provides the pure domain orchestrator for Kubernetes Workload Provisioning.
//
// Architectural Blueprint:
// - Step 06 Service: Coordinates business rules, state machines, workflows, and abstract ports.
// - Enforces OpenTelemetry child tracing, tenant isolation, and Inversion of Control.
// - Depends strictly on abstract outbound ports (WorkloadStorePort, K8sClientPort, GitOpsDeliveryPort, KmsEncryptionPort).
// - Zero inline comments inside method bodies; execution is traced and returns typed errors.
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/machines"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/workflows"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/ports"
	pacports "internal-developer-platform/src/features/policy-as-code/infrastructure/ports"
	regimerules "internal-developer-platform/src/features/platform-regime/domain/rules"
	regimeports "internal-developer-platform/src/features/platform-regime/infrastructure/ports"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

const (
	AttrAdmissionExceptionApplied = "admission.exception_applied"
	AttrAdmissionExceptionID      = "admission.exception_id"
)

type WorkloadProvisioningService struct {
	store          ports.WorkloadStorePort
	k8sClient      ports.K8sClientPort
	policyRepo     pacports.PolicyRepositoryPort
	regimeRepo     regimeports.PlatformRegimeRepositoryPort
	gitopsDelivery ports.GitOpsDeliveryPort
	kmsPort        ports.KmsEncryptionPort
}

func NewWorkloadProvisioningService(store ports.WorkloadStorePort, k8sClient ports.K8sClientPort, policyRepo ...pacports.PolicyRepositoryPort) *WorkloadProvisioningService {
	var pr pacports.PolicyRepositoryPort
	if len(policyRepo) > 0 {
		pr = policyRepo[0]
	}
	return &WorkloadProvisioningService{
		store:      store,
		k8sClient:  k8sClient,
		policyRepo: pr,
	}
}

func (s *WorkloadProvisioningService) WithRegimeRepository(regimeRepo regimeports.PlatformRegimeRepositoryPort) *WorkloadProvisioningService {
	s.regimeRepo = regimeRepo
	return s
}


func (s *WorkloadProvisioningService) WithKMSPort(kmsPort ports.KmsEncryptionPort) *WorkloadProvisioningService {
	s.kmsPort = kmsPort
	return s
}

func (s *WorkloadProvisioningService) WithGitOpsDelivery(gitopsDelivery ports.GitOpsDeliveryPort) *WorkloadProvisioningService {
	s.gitopsDelivery = gitopsDelivery
	return s
}

func NewService(store ports.WorkloadStorePort, k8sClient ports.K8sClientPort, policyRepo ...pacports.PolicyRepositoryPort) *WorkloadProvisioningService {
	return NewWorkloadProvisioningService(store, k8sClient, policyRepo...)
}

func (s *WorkloadProvisioningService) checkIdempotencyReplay(ctx context.Context, cmd contracts.WorkloadProvisionCommand) (*contracts.WorkloadProvisionResult, error) {
	existing, err := s.store.GetByIdempotencyKey(ctx, cmd.TenantID, cmd.IdempotencyKey)
	if err != nil || existing == nil {
		return nil, err
	}
	var bundle contracts.ManifestBundle
	if applied, bErr := s.k8sClient.GetAppliedManifests(ctx, existing.Namespace, existing.WorkloadName); bErr == nil && applied != nil {
		bundle = *applied
	}
	return &contracts.WorkloadProvisionResult{
		Provision:               *existing,
		InjectedSecurityContext: true,
		InjectedProbes:          true,
		PDBCreated:              existing.Replicas >= 2,
		Manifests:               bundle,
		Admission: contracts.AdmissionReport{
			Admitted:   true,
			Violations: nil,
		},
		Message: "idempotent replay",
	}, nil
}

func (s *WorkloadProvisioningService) verifyQuotaHeadroom(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) error {
	return observability.WithSpan(ctx, "kubernetes.workload.verify_quota_headroom", func(ctx context.Context) error {
		quota, err := s.k8sClient.GetNamespaceQuota(ctx, cmd.Namespace)
		if err != nil {
			return err
		}
		if quota != nil {
			headroom := rules.EvaluateResourceQuotaSurgeHeadroom(*quota, mutations.CPURequest, mutations.MemoryRequest, cmd.Replicas, cmd.Strategy)
			if !headroom.SufficientHeadroom {
				return errors.NewConflictError("insufficient quota headroom: " + headroom.ViolationMessage)
			}
		}
		return nil
	}, attribute.String("namespace", cmd.Namespace))
}

func (s *WorkloadProvisioningService) evaluateAdmission(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) (contracts.AdmissionReport, error) {
	return observability.RunTraced(ctx, "kubernetes.workload.validate_admission", func(ctx context.Context) (contracts.AdmissionReport, error) {
		report, err := rules.ValidateAdmission(cmd, mutations)
		if err == nil {
			return report, nil
		}
		if s.policyRepo != nil {
			activeExceptions, excErr := s.policyRepo.GetActiveExceptionsForWorkload(ctx, cmd.TenantID, cmd.Namespace, cmd.WorkloadName)
			if excErr == nil && len(activeExceptions) > 0 {
				matching := activeExceptions[0]
				return contracts.AdmissionReport{
					Admitted:         true,
					Violations:       report.Violations,
					ExceptionApplied: true,
					ExceptionID:      matching.PublicID,
					ApproverID:       matching.ApproverID,
				}, nil
			}
		}
		return report, err
	})
}

func (s *WorkloadProvisioningService) advanceStateMachine(ctx context.Context, record *contracts.WorkloadProvisionRecord) error {
	return observability.WithSpan(ctx, "kubernetes.workload.state_machine_transition", func(ctx context.Context) error {
		vStatus, err := machines.TransitionWorkloadState(record.Status, machines.EventValidate)
		if err != nil {
			return err
		}
		aStatus, err := machines.TransitionWorkloadState(vStatus, machines.EventAdmit)
		if err != nil {
			return err
		}
		pStatus, err := machines.TransitionWorkloadState(aStatus, machines.EventApply)
		if err != nil {
			return err
		}
		record.Status = pStatus
		record.UpdatedAt = time.Now().UTC()
		if err := s.store.Update(ctx, *record); err != nil {
			return err
		}
		record.Version = 2
		return nil
	})
}


func (s *WorkloadProvisioningService) applyWorkloadResources(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations, record *contracts.WorkloadProvisionRecord) error {
	return observability.WithSpan(ctx, "kubernetes.workload.apply_resources", func(ctx context.Context) error {
		if s.gitopsDelivery != nil {
			bundle, err := s.k8sClient.GenerateManifestBundle(ctx, cmd, mutations)
			if err != nil {
				return err
			}
			if err := s.gitopsDelivery.CommitManifestBundle(ctx, cmd.TenantID, cmd.WorkloadName, *bundle); err != nil {
				rej, _ := machines.TransitionWorkloadState(record.Status, machines.EventReject)
				record.Status = rej
				record.Version = 2
				_ = s.store.Update(ctx, *record)
				return errors.NewConflictError("gitops manifest commit failed: " + err.Error())
			}
			return nil
		}
		return s.applyDirectKubernetesResources(ctx, cmd, mutations, record)
	})
}

func (s *WorkloadProvisioningService) applyDirectKubernetesResources(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations, record *contracts.WorkloadProvisionRecord) error {
	if err := s.k8sClient.ApplyDeployment(ctx, cmd, mutations); err != nil {
		rej, _ := machines.TransitionWorkloadState(record.Status, machines.EventReject)
		record.Status = rej
		record.Version = 2
		_ = s.store.Update(ctx, *record)
		return errors.NewConflictError("kubernetes deployment apply failed: " + err.Error())
	}
	if mutations.PDBRequired {
		if err := s.k8sClient.ApplyPDB(ctx, cmd.Namespace, cmd.WorkloadName, mutations.MinAvailable); err != nil {
			rej, _ := machines.TransitionWorkloadState(record.Status, machines.EventReject)
			record.Status = rej
			record.Version = 2
			_ = s.store.Update(ctx, *record)
			return errors.NewConflictError("kubernetes pdb apply failed: " + err.Error())
		}
	}
	return nil
}

func (s *WorkloadProvisioningService) observeWorkloadStatus(ctx context.Context, cmd contracts.WorkloadProvisionCommand, record *contracts.WorkloadProvisionRecord) error {
	return observability.WithSpan(ctx, "kubernetes.workload.observe_readiness", func(ctx context.Context) error {
		ready, _ := s.k8sClient.ObserveWorkloadReadiness(ctx, cmd.Namespace, cmd.WorkloadName, cmd.Replicas)
		if ready {
			if obs, err := machines.TransitionWorkloadState(record.Status, machines.EventReplicasReady); err == nil {
				record.Status = obs
				record.UpdatedAt = time.Now().UTC()
				if err := s.store.Update(ctx, *record); err != nil {
					return err
				}
				record.Version = 3
				return nil
			}

		}
		return nil
	})
}

func resolveApprovalBinding(cmd contracts.WorkloadProvisionCommand, specHash string) (string, string) {
	approved := cmd.ApprovedSpecHash
	if approved == "" {
		approved = specHash
	}
	role := cmd.ApproverRole
	if role == "" {
		role = "system-auto"
	}
	return approved, role
}

func buildInitialRecord(cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) contracts.WorkloadProvisionRecord {
	strategy := cmd.Strategy
	if strategy == "" {
		strategy = contracts.StrategyCanary
	}
	specHash := contracts.ComputeWorkloadSpecHash(cmd)
	approvedHash, approverRole := resolveApprovalBinding(cmd, specHash)
	tier := cmd.IncidentTier
	if tier == "" {
		tier = contracts.IncidentTier2
	}
	return contracts.WorkloadProvisionRecord{
		ID:                      cmd.ID,
		PublicID:                derivePublicID(cmd.PublicID, cmd.ID),
		TenantID:                cmd.TenantID,
		Source:                  deriveFallback(cmd.Source, "api"),
		ExternalID:              deriveFallback(cmd.ExternalID, cmd.ID),
		CorrelationID:           deriveCorrelationID(cmd.CorrelationID),
		SpecHash:                specHash,
		ApprovedSpecHash:        approvedHash,
		ApproverRole:            approverRole,
		ImagePullPolicy:         mutations.ImagePullPolicy,
		BreakglassJustification: cmd.BreakglassJustification,
		ServiceID:               cmd.ServiceID,
		Namespace:               cmd.Namespace,
		WorkloadName:            cmd.WorkloadName,
		CPURequest:              mutations.CPURequest,
		CPULimit:                mutations.CPULimit,
		MemoryRequest:           mutations.MemoryRequest,
		MemoryLimit:             mutations.MemoryLimit,
		Replicas:                cmd.Replicas,
		Strategy:                strategy,
		Status:                  contracts.StatusSubmitted,
		EnableDataAccessLogs:   cmd.AuditLogging.EnableDataAccessLogs,
		AuditDestinationBucket: cmd.AuditLogging.DestinationBucket,
		AuditRetentionDays:     cmd.AuditLogging.RetentionDays,
		AuditLockedRetention:   cmd.AuditLogging.LockedRetention,
		OnCallPager:            cmd.OnCallPager,
		RunbookURL:             cmd.RunbookURL,
		IncidentTier:           tier,
		IdempotencyKey:          cmd.IdempotencyKey,
		Version:                 1,
		CreatedAt:               time.Now().UTC(),
		UpdatedAt:               time.Now().UTC(),
	}
}

func deriveCorrelationID(correlationID string) string {
	if _, err := uuid.Parse(correlationID); err == nil {
		return correlationID
	}
	return uuid.New().String()
}

func derivePublicID(explicitID, fallbackID string) string {
	if explicitID != "" {
		return explicitID
	}
	clean := strings.ReplaceAll(fallbackID, "-", "")
	if len(clean) > 26 {
		clean = clean[:26]
	}
	return fmt.Sprintf("wld_%s", clean)
}

func deriveFallback(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

func mergeCanaryBundle(bundle *contracts.ManifestBundle, canaryBundle *workflows.SynthesizedCanaryBundle) contracts.ManifestBundle {
	if canaryBundle == nil {
		return *bundle
	}
	bundle.RolloutYAML = canaryBundle.RolloutYAML
	bundle.AnalysisTemplateYAML = canaryBundle.AnalysisTemplateYAML
	bundle.VirtualServiceYAML = canaryBundle.VirtualServiceYAML
	bundle.DestinationRuleYAML = canaryBundle.DestinationRuleYAML
	bundle.WorkloadIdentityYAML = canaryBundle.WorkloadIdentityYAML
	bundle.DatabaseYAML = canaryBundle.DatabaseYAML
	bundle.HTTPRouteYAML = canaryBundle.HTTPRouteYAML
	bundle.BackendPolicyYAML = canaryBundle.BackendPolicyYAML
	bundle.SecurityPolicyYAML = canaryBundle.SecurityPolicyYAML
	bundle.BackupPlanYAML = canaryBundle.BackupPlanYAML
	bundle.RestorePlanYAML = canaryBundle.RestorePlanYAML
	return *bundle
}

func (s *WorkloadProvisioningService) executePipeline(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations, admissionReport contracts.AdmissionReport) (*contracts.WorkloadProvisionResult, error) {
	_, canaryBundle, err := workflows.ExecuteProvisionDAG(ctx, cmd, mutations)
	if err != nil {
		return nil, err
	}
	record := buildInitialRecord(cmd, mutations)
	if err := s.store.Save(ctx, record); err != nil {
		return nil, err
	}
	if err := s.advanceStateMachine(ctx, &record); err != nil {
		return nil, err
	}
	if err := s.applyWorkloadResources(ctx, cmd, mutations, &record); err != nil {
		return nil, err
	}
	_ = s.observeWorkloadStatus(ctx, cmd, &record)
	bundle, err := s.k8sClient.GenerateManifestBundle(ctx, cmd, mutations)
	if err != nil {
		return nil, err
	}
	return &contracts.WorkloadProvisionResult{
		Provision:               record,
		InjectedSecurityContext: true,
		InjectedProbes:          true,
		PDBCreated:              mutations.PDBRequired,
		Manifests:               mergeCanaryBundle(bundle, canaryBundle),
		Admission:               admissionReport,
		Message:                 "workload provisioned and admitted successfully",
	}, nil
}

func (s *WorkloadProvisioningService) ProvisionWorkload(ctx context.Context, cmd contracts.WorkloadProvisionCommand) (*contracts.WorkloadProvisionResult, error) {
	return observability.RunTraced(ctx, "kubernetes.workload.provision", func(ctx context.Context) (*contracts.WorkloadProvisionResult, error) {
		if err := contracts.ValidateWorkloadCommand(cmd); err != nil {
			return nil, err
		}
		if err := rules.ValidateSpecApproval(cmd); err != nil {
			return nil, err
		}
		if err := rules.ValidateBackupPlanRules(cmd); err != nil {
			return nil, err
		}
		if replay, err := s.checkIdempotencyReplay(ctx, cmd); replay != nil || err != nil {
			return replay, err
		}
		mutations := rules.EvaluateWorkloadMutations(cmd)
		if err := s.verifyQuotaHeadroom(ctx, cmd, mutations); err != nil {
			return nil, err
		}
		report, err := s.evaluateAdmission(ctx, cmd, mutations)
		if err != nil {
			return nil, err
		}
		if err := s.validatePlatformRegime(ctx, cmd); err != nil {
			return nil, err
		}
		if err := s.encryptWorkloadSecrets(ctx, &cmd); err != nil {
			return nil, err
		}

		return s.executePipeline(ctx, cmd, mutations, report)
	}, attribute.String("tenant.id", cmd.TenantID), attribute.String("workload.name", cmd.WorkloadName))
}

func (s *WorkloadProvisioningService) validatePlatformRegime(ctx context.Context, cmd contracts.WorkloadProvisionCommand) error {
	if s.regimeRepo == nil {
		return nil
	}
	regime, err := s.regimeRepo.GetActive(ctx, cmd.TenantID)
	if err != nil || regime == nil {
		return nil
	}
	ok, violations := regimerules.ValidateWorkloadAgainstRegime(cmd, *regime)
	if !ok && len(violations) > 0 {
		return errors.NewValidationError(fmt.Sprintf("platform regime policy violation: %s (%s)", violations[0].Message, violations[0].Rule))
	}
	return nil
}

func (s *WorkloadProvisioningService) encryptWorkloadSecrets(ctx context.Context, cmd *contracts.WorkloadProvisionCommand) error {
	if s.kmsPort == nil || len(cmd.EncryptedSecrets) == 0 {
		return nil
	}
	encrypted := make(map[string]string)
	for k, v := range cmd.EncryptedSecrets {
		ciphertext, err := s.kmsPort.EncryptSecret(ctx, cmd.KMSKeyURI, []byte(v), cmd.TenantID)
		if err != nil {
			return err
		}
		encrypted[k] = ciphertext
	}
	cmd.EncryptedSecrets = encrypted
	return nil
}

func (s *WorkloadProvisioningService) GetWorkload(ctx context.Context, tenantID string, id string) (*contracts.WorkloadProvisionRecord, error) {
	return observability.RunTraced(ctx, "kubernetes.workload.get", func(ctx context.Context) (*contracts.WorkloadProvisionRecord, error) {
		if tenantID == "" || id == "" {
			return nil, errors.NewValidationError("tenantId and id are required")
		}
		return s.store.GetByID(ctx, tenantID, id)
	}, attribute.String("tenant.id", tenantID), attribute.String("workload.id", id))
}

func (s *WorkloadProvisioningService) ListWorkloads(ctx context.Context, filter contracts.FilterCriteria) ([]contracts.WorkloadProvisionRecord, error) {
	return observability.RunTraced(ctx, "kubernetes.workload.list", func(ctx context.Context) ([]contracts.WorkloadProvisionRecord, error) {
		if filter.TenantID == "" {
			return nil, errors.NewValidationError("tenantId is required")
		}
		return s.store.List(ctx, filter)
	}, attribute.String("tenant.id", filter.TenantID))
}

func (s *WorkloadProvisioningService) AwaitWorkloadReadiness(ctx context.Context, tenantID, id string, timeout time.Duration) (*contracts.WorkloadProvisionRecord, error) {
	return observability.RunTraced(ctx, "kubernetes.workload.await_readiness", func(ctx context.Context) (*contracts.WorkloadProvisionRecord, error) {
		record, err := s.store.GetByID(ctx, tenantID, id)
		if err != nil || record == nil {
			return nil, errors.NewNotFoundError("workload record not found")
		}
		if record.Status == contracts.StatusObservedReady || record.Status == contracts.StatusActive {
			return record, nil
		}
		return s.pollWorkloadReadiness(ctx, record, timeout)
	})
}

func (s *WorkloadProvisioningService) pollWorkloadReadiness(ctx context.Context, record *contracts.WorkloadProvisionRecord, timeout time.Duration) (*contracts.WorkloadProvisionRecord, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return s.markWorkloadDegraded(ctx, record, ctx.Err())
		case now := <-ticker.C:
			if now.After(deadline) {
				return s.markWorkloadDegraded(ctx, record, errors.NewDeadlineExceededError("workload readiness timeout exceeded"))
			}
			ready, _ := s.k8sClient.ObserveWorkloadReadiness(ctx, record.Namespace, record.WorkloadName, record.Replicas)
			if ready {
				return s.markWorkloadReady(ctx, record)
			}
		}
	}
}

func (s *WorkloadProvisioningService) markWorkloadReady(ctx context.Context, record *contracts.WorkloadProvisionRecord) (*contracts.WorkloadProvisionRecord, error) {
	obs, err := machines.TransitionWorkloadState(record.Status, machines.EventReplicasReady)
	if err != nil {
		return record, err
	}
	record.Status = obs
	record.Version++
	if err := s.store.Update(ctx, *record); err != nil {
		return nil, err
	}
	return record, nil
}

func (s *WorkloadProvisioningService) markWorkloadDegraded(ctx context.Context, record *contracts.WorkloadProvisionRecord, cause error) (*contracts.WorkloadProvisionRecord, error) {
	deg, err := machines.TransitionWorkloadState(record.Status, machines.EventDegrade)
	if err != nil {
		return record, cause
	}
	record.Status = deg
	record.Version++
	_ = s.store.Update(ctx, *record)
	return record, cause
}

func (s *WorkloadProvisioningService) RollbackWorkload(ctx context.Context, tenantID, workloadID, reason string) (*contracts.WorkloadProvisionRecord, error) {
	return observability.RunTraced(ctx, "kubernetes.workload.rollback", func(ctx context.Context) (*contracts.WorkloadProvisionRecord, error) {
		record, err := s.store.GetByID(ctx, tenantID, workloadID)
		if err != nil || record == nil {
			return nil, errors.NewNotFoundError("workload record not found")
		}
		if s.gitopsDelivery != nil {
			if rErr := s.gitopsDelivery.RevertManifestBundle(ctx, tenantID, record.WorkloadName, reason); rErr != nil {
				return nil, errors.NewConflictError("gitops revert failed: " + rErr.Error())
			}
		}
		newStatus, err := machines.TransitionWorkloadState(record.Status, machines.EventDegrade)
		if err != nil {
			return nil, err
		}
		record.Status = newStatus
		record.Version++
		record.UpdatedAt = time.Now().UTC()
		if uErr := s.store.Update(ctx, *record); uErr != nil {
			return nil, uErr
		}
		return record, nil
	})
}

func (s *WorkloadProvisioningService) DecommissionWorkload(ctx context.Context, cmd contracts.WorkloadDecommissionCommand) (*contracts.DecommissionExecutionResult, error) {
	return observability.RunTraced(ctx, "kubernetes.workload.decommission", func(ctx context.Context) (*contracts.DecommissionExecutionResult, error) {
		if err := rules.ValidateDecommissionDecision(cmd); err != nil {
			return nil, err
		}

		records, err := s.store.List(ctx, contracts.FilterCriteria{
			TenantID: cmd.TenantID,
			Limit:    100,
		})
		if err != nil {
			return nil, err
		}

		var targetRecord *contracts.WorkloadProvisionRecord
		for _, r := range records {
			if r.Namespace == cmd.Namespace && r.WorkloadName == cmd.WorkloadName {
				rec := r
				targetRecord = &rec
				break
			}
		}

		if targetRecord == nil {
			return nil, errors.NewNotFoundError(fmt.Sprintf("workload %s/%s not found for decommissioning", cmd.Namespace, cmd.WorkloadName))
		}

		decommStatus, err := machines.TransitionWorkloadState(targetRecord.Status, machines.EventDecommission)
		if err != nil {
			return nil, err
		}
		targetRecord.Status = decommStatus
		targetRecord.Version++
		targetRecord.UpdatedAt = time.Now().UTC()
		if err := s.store.Update(ctx, *targetRecord); err != nil {
			return nil, err
		}

		executedSteps, err := workflows.ExecuteDecommissionDAG(ctx, cmd)
		if err != nil {
			_, _ = machines.TransitionWorkloadState(targetRecord.Status, machines.EventReject)
			return nil, err
		}

		finalStatus, err := machines.TransitionWorkloadState(targetRecord.Status, machines.EventCompleteDecommission)
		if err != nil {
			return nil, err
		}
		targetRecord.Status = finalStatus
		targetRecord.Version++
		targetRecord.UpdatedAt = time.Now().UTC()
		if err := s.store.Update(ctx, *targetRecord); err != nil {
			return nil, err
		}

		auditRecordID := fmt.Sprintf("decomm-audit-%s", uuid.New().String())
		return &contracts.DecommissionExecutionResult{
			WorkloadName:  cmd.WorkloadName,
			Namespace:     cmd.Namespace,
			Status:        finalStatus,
			StepsExecuted: executedSteps,
			AuditRecordID: auditRecordID,
		}, nil
	}, attribute.String("tenant.id", cmd.TenantID), attribute.String("workload.name", cmd.WorkloadName))
}
