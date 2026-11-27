// Package service provides domain orchestration for Service Catalog.
//
// Architectural Blueprint:
// - Step 06 Service: Domain orchestrator for service registry and discovery.
// - Enforces OpenTelemetry child span propagation via observability.RunTraced.
// - 100% comment-free function bodies.
package service

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"

	catalogContracts "internal-developer-platform/src/features/service-catalog/internal/contracts"
	catalogPorts "internal-developer-platform/src/features/service-catalog/internal/infrastructure/ports"
	"internal-developer-platform/src/infra/observability"
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
	return observability.RunTraced(ctx, "service_catalog.service.register", func(ctx context.Context) (*catalogContracts.ServiceDescriptor, error) {
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
	}, attribute.String("tenant_id", input.TenantID), attribute.String("service_id", input.ID))
}

func (s *ServiceCatalogService) GetService(ctx context.Context, tenantID string, id string) (*catalogContracts.ServiceDescriptor, error) {
	return observability.RunTraced(ctx, "service_catalog.service.get", func(ctx context.Context) (*catalogContracts.ServiceDescriptor, error) {
		if tenantID == "" || id == "" {
			return nil, errors.NewValidationError("tenantId and id are required")
		}
		return s.repo.GetServiceByID(ctx, tenantID, id)
	}, attribute.String("tenant_id", tenantID), attribute.String("service_id", id))
}
