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
		return &contracts.WorkloadProvisionResult{
			Provision:               *existing,
			InjectedSecurityContext: true,
			InjectedProbes:          true,
			PDBCreated:              existing.Replicas >= 2,
			Message:                 "idempotent replay",
		}, nil
	}

	mutations := rules.EvaluateWorkloadMutations(cmd)
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

	activeStatus, err := machines.TransitionWorkloadState(record.Status, machines.EventActivate)
	if err != nil {
		return nil, err
	}
	record.Status = activeStatus
	record.Version = 2

	if err := s.store.Update(ctx, record); err != nil {
		return nil, err
	}

	return &contracts.WorkloadProvisionResult{
		Provision:               record,
		InjectedSecurityContext: true,
		InjectedProbes:          true,
		PDBCreated:              mutations.PDBRequired,
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

