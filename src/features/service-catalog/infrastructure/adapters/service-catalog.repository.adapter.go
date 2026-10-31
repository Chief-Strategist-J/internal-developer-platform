package adapters

import (
	"context"
	"sync"

	catalogContracts "internal-developer-platform/src/features/service-catalog/contracts"
	"internal-developer-platform/src/shared/errors"
)

type MemoryServiceCatalogAdapter struct {
	mu    sync.RWMutex
	store map[string]*catalogContracts.ServiceDescriptor
}

func NewMemoryServiceCatalogAdapter() *MemoryServiceCatalogAdapter {
	return &MemoryServiceCatalogAdapter{
		store: make(map[string]*catalogContracts.ServiceDescriptor),
	}
}

func (a *MemoryServiceCatalogAdapter) GetServiceByID(ctx context.Context, tenantID string, id string) (*catalogContracts.ServiceDescriptor, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	key := tenantID + ":" + id
	svc, exists := a.store[key]
	if !exists {
		return nil, errors.NewNotFoundError("service not found in catalog")
	}
	return svc, nil
}

func (a *MemoryServiceCatalogAdapter) RegisterService(ctx context.Context, service *catalogContracts.ServiceDescriptor) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := service.TenantID + ":" + service.ID
	if _, exists := a.store[key]; exists {
		return errors.NewConflictError("service already exists in catalog")
	}
	a.store[key] = service
	return nil
}
