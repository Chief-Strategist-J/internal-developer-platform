package service

import (
	"context"
	"time"

	catalogContracts "internal-developer-platform/src/features/service-catalog/contracts"
	catalogPorts "internal-developer-platform/src/features/service-catalog/infrastructure/ports"
	"internal-developer-platform/src/shared/errors"
)

type ServiceCatalogService struct {
	repo catalogPorts.ServiceCatalogRepositoryPort
}

func NewServiceCatalogService(repo catalogPorts.ServiceCatalogRepositoryPort) *ServiceCatalogService {
	return &ServiceCatalogService{
		repo: repo,
	}
}

func (s *ServiceCatalogService) RegisterService(ctx context.Context, input catalogContracts.RegisterServiceInput) (*catalogContracts.ServiceDescriptor, error) {
	if err := catalogContracts.ValidateRegisterServiceInput(input); err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	lifecycle := input.Lifecycle
	if lifecycle == "" {
		lifecycle = "production"
	}
	descriptor := &catalogContracts.ServiceDescriptor{
		ID:        input.ID,
		TenantID:  input.TenantID,
		Name:      input.Name,
		Owner:     input.Owner,
		Lifecycle: lifecycle,
		Status:    catalogContracts.ServiceStatusActive,
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.RegisterService(ctx, descriptor); err != nil {
		return nil, err
	}
	return descriptor, nil
}

func (s *ServiceCatalogService) GetService(ctx context.Context, tenantID string, id string) (*catalogContracts.ServiceDescriptor, error) {
	if tenantID == "" || id == "" {
		return nil, errors.NewValidationError("tenantId and id are required")
	}
	return s.repo.GetServiceByID(ctx, tenantID, id)
}
