package service

import (
	"context"
	"time"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/machines"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/workflows"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/ports"
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
	if err := contracts.ValidateWorkloadCommand(cmd); err != nil {
		return nil, err
	}

	existing, err := s.store.GetByIdempotencyKey(ctx, cmd.TenantID, cmd.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
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

	mutations := rules.EvaluateWorkloadMutations(cmd)

	quota, qErr := s.k8sClient.GetNamespaceQuota(ctx, cmd.Namespace)
	if qErr == nil && quota != nil {
		headroom := rules.EvaluateResourceQuotaHeadroom(*quota, mutations.CPURequest, mutations.MemoryRequest, cmd.Replicas)
		if !headroom.SufficientHeadroom {
			return nil, errors.NewConflictError("insufficient quota headroom: " + headroom.ViolationMessage)
		}
	}

	admissionReport, err := rules.ValidateAdmission(cmd, mutations)
	if err != nil {
		return nil, err
	}

	_ = workflows.ExecuteProvisionDAG(ctx, cmd, mutations)

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
		Status:         contracts.StatusSubmitted,
		IdempotencyKey: cmd.IdempotencyKey,
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.store.Save(ctx, record); err != nil {
		return nil, err
	}

	validatedStatus, err := machines.TransitionWorkloadState(record.Status, machines.EventValidate)
	if err != nil {
		return nil, err
	}
	record.Status = validatedStatus

	admittedStatus, err := machines.TransitionWorkloadState(record.Status, machines.EventAdmit)
	if err != nil {
		return nil, err
	}
	record.Status = admittedStatus

	provisioningStatus, err := machines.TransitionWorkloadState(record.Status, machines.EventApply)
	if err != nil {
		return nil, err
	}
	record.Status = provisioningStatus

	if err := s.k8sClient.ApplyDeployment(ctx, cmd, mutations); err != nil {
		rejectedStatus, _ := machines.TransitionWorkloadState(record.Status, machines.EventReject)
		record.Status = rejectedStatus
		record.Version = 2
		_ = s.store.Update(ctx, record)
		return nil, errors.NewConflictError("kubernetes deployment apply failed: " + err.Error())
	}

	if mutations.PDBRequired {
		if err := s.k8sClient.ApplyPDB(ctx, cmd.Namespace, cmd.WorkloadName, mutations.MinAvailable); err != nil {
			rejectedStatus, _ := machines.TransitionWorkloadState(record.Status, machines.EventReject)
			record.Status = rejectedStatus
			record.Version = 2
			_ = s.store.Update(ctx, record)
			return nil, errors.NewConflictError("kubernetes pdb apply failed: " + err.Error())
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
		return nil, err
	}

	bundle, err := s.k8sClient.GenerateManifestBundle(ctx, cmd, mutations)
	if err != nil {
		return nil, err
	}

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
	if tenantID == "" || id == "" {
		return nil, errors.NewValidationError("tenantId and id are required")
	}
	return s.store.GetByID(ctx, tenantID, id)
}

func (s *WorkloadProvisioningService) ListWorkloads(ctx context.Context, filter contracts.FilterCriteria) ([]contracts.WorkloadProvisionRecord, error) {
	if filter.TenantID == "" {
		return nil, errors.NewValidationError("tenantId is required")
	}
	return s.store.List(ctx, filter)
}
