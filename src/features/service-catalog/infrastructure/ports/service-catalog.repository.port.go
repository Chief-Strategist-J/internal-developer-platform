package ports

import (
	"context"

	catalogContracts "internal-developer-platform/src/features/service-catalog/contracts"
)

type ServiceCatalogRepositoryPort interface {
	GetServiceByID(ctx context.Context, tenantID string, id string) (*catalogContracts.ServiceDescriptor, error)
	RegisterService(ctx context.Context, service *catalogContracts.ServiceDescriptor) error
}
