package service

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/machines"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/workflows"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/ports"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type WorkloadProvisioningService struct {
	store     ports.WorkloadStorePort
	k8sClient ports.K8sClientPort
}

func NewWorkloadProvisioningService(store ports.WorkloadStorePort, k8sClient ports.K8sClientPort) *WorkloadProvisioningService {
	return &WorkloadProvisioningService{
		store:     store,
		k8sClient: k8sClient,
	}
}

func (s *WorkloadProvisioningService) ProvisionWorkload(ctx context.Context, cmd contracts.WorkloadProvisionCommand) (*contracts.WorkloadProvisionResult, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "kubernetes.workload.provision",
		attribute.String("tenant.id", cmd.TenantID),
		attribute.String("service.id", cmd.ServiceID),
		attribute.String("workload.name", cmd.WorkloadName),
		attribute.String("namespace", cmd.Namespace),
	)
	defer span.End()

	if err := contracts.ValidateWorkloadCommand(cmd); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	existing, err := s.store.GetByIdempotencyKey(ctx, cmd.TenantID, cmd.IdempotencyKey)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	if existing != nil {
		var bundle contracts.ManifestBundle
		if applied, bErr := s.k8sClient.GetAppliedManifests(ctx, existing.Namespace, existing.WorkloadName); bErr == nil && applied != nil {
			bundle = *applied
		}
		span.SetAttributes(attribute.Bool("idempotent.replay", true))
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

	mutations := rules.EvaluateWorkloadMutations(cmd)

	quota, qErr := s.k8sClient.GetNamespaceQuota(ctx, cmd.Namespace)
	if qErr == nil && quota != nil {
		headroom := rules.EvaluateResourceQuotaHeadroom(*quota, mutations.CPURequest, mutations.MemoryRequest, cmd.Replicas)
		if !headroom.SufficientHeadroom {
			quotaErr := errors.NewConflictError("insufficient quota headroom: " + headroom.ViolationMessage)
			observability.RecordError(span, quotaErr)
			return nil, quotaErr
		}
	}

	admissionReport, err := rules.ValidateAdmission(cmd, mutations)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	_ = workflows.ExecuteProvisionDAG(ctx, cmd, mutations)

	strategy := cmd.Strategy
	if strategy == "" {
		strategy = contracts.StrategyCanary
	}

	now := time.Now().UTC()
	record := contracts.WorkloadProvisionRecord{
		ID:             cmd.ID,
		TenantID:       cmd.TenantID,
		ServiceID:      cmd.ServiceID,
		Namespace:      cmd.Namespace,
		WorkloadName:   cmd.WorkloadName,
		CPURequest:     mutations.CPURequest,
		CPULimit:       mutations.CPULimit,
		MemoryRequest:  mutations.MemoryRequest,
		MemoryLimit:    mutations.MemoryLimit,
		Replicas:       cmd.Replicas,
		Strategy:       strategy,
		Status:         contracts.StatusSubmitted,
		IdempotencyKey: cmd.IdempotencyKey,
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.store.Save(ctx, record); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	validatedStatus, err := machines.TransitionWorkloadState(record.Status, machines.EventValidate)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	record.Status = validatedStatus

	admittedStatus, err := machines.TransitionWorkloadState(record.Status, machines.EventAdmit)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	record.Status = admittedStatus

	provisioningStatus, err := machines.TransitionWorkloadState(record.Status, machines.EventApply)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	record.Status = provisioningStatus

	if err := s.k8sClient.ApplyDeployment(ctx, cmd, mutations); err != nil {
		rejectedStatus, _ := machines.TransitionWorkloadState(record.Status, machines.EventReject)
		record.Status = rejectedStatus
		record.Version = 2
		_ = s.store.Update(ctx, record)
		k8sErr := errors.NewConflictError("kubernetes deployment apply failed: " + err.Error())
		observability.RecordError(span, k8sErr)
		return nil, k8sErr
	}

	if mutations.PDBRequired {
		if err := s.k8sClient.ApplyPDB(ctx, cmd.Namespace, cmd.WorkloadName, mutations.MinAvailable); err != nil {
			rejectedStatus, _ := machines.TransitionWorkloadState(record.Status, machines.EventReject)
			record.Status = rejectedStatus
			record.Version = 2
			_ = s.store.Update(ctx, record)
			pdbErr := errors.NewConflictError("kubernetes pdb apply failed: " + err.Error())
			observability.RecordError(span, pdbErr)
			return nil, pdbErr
		}
	}

	ready, _ := s.k8sClient.ObserveWorkloadReadiness(ctx, cmd.Namespace, cmd.WorkloadName, cmd.Replicas)
	if ready {
		observedStatus, err := machines.TransitionWorkloadState(record.Status, machines.EventReplicasReady)
		if err == nil {
			record.Status = observedStatus
		}
	} else {
		degradedStatus, err := machines.TransitionWorkloadState(record.Status, machines.EventDegrade)
		if err == nil {
			record.Status = degradedStatus
		}
	}
	record.Version = 2

	if err := s.store.Update(ctx, record); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	bundle, err := s.k8sClient.GenerateManifestBundle(ctx, cmd, mutations)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}

	span.SetAttributes(
		attribute.String("provision.status", string(record.Status)),
		attribute.Bool("pdb.created", mutations.PDBRequired),
	)

	return &contracts.WorkloadProvisionResult{
		Provision:               record,
		InjectedSecurityContext: true,
		InjectedProbes:          true,
		PDBCreated:              mutations.PDBRequired,
		Manifests:               *bundle,
		Admission:               admissionReport,
		Message:                 "workload provisioned and admitted successfully",
	}, nil
}

func (s *WorkloadProvisioningService) GetWorkload(ctx context.Context, tenantID string, id string) (*contracts.WorkloadProvisionRecord, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "kubernetes.workload.get",
		attribute.String("tenant.id", tenantID),
		attribute.String("workload.id", id),
	)
	defer span.End()

	if tenantID == "" || id == "" {
		err := errors.NewValidationError("tenantId and id are required")
		observability.RecordError(span, err)
		return nil, err
	}
	rec, err := s.store.GetByID(ctx, tenantID, id)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	return rec, nil
}

func (s *WorkloadProvisioningService) ListWorkloads(ctx context.Context, filter contracts.FilterCriteria) ([]contracts.WorkloadProvisionRecord, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "kubernetes.workload.list",
		attribute.String("tenant.id", filter.TenantID),
	)
	defer span.End()

	if filter.TenantID == "" {
		err := errors.NewValidationError("tenantId is required")
		observability.RecordError(span, err)
		return nil, err
	}
	records, err := s.store.List(ctx, filter)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	return records, nil
}
