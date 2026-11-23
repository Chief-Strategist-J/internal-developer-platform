// Package adapters provides in-memory repository adapters for Service Catalog.
//
// Architectural Blueprint:
// - Step 11 Adapter: In-memory repository adapter for Service Catalog.
// - Traced under OpenTelemetry child spans with error recording.
// - 100% comment-free function bodies.
package adapters

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel/attribute"

	catalogContracts "internal-developer-platform/src/features/service-catalog/contracts"
	"internal-developer-platform/src/infra/observability"
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
	return observability.RunTraced(ctx, "memory.catalog.get_by_id", func(ctx context.Context) (*catalogContracts.ServiceDescriptor, error) {
		a.mu.RLock()
		defer a.mu.RUnlock()
		key := tenantID + ":" + id
		svc, exists := a.store[key]
		if !exists {
			return nil, errors.NewNotFoundError("service not found in catalog")
		}
		return svc, nil
	}, attribute.String("db.system", "in-memory"), attribute.String("tenant_id", tenantID), attribute.String("service_id", id))
}

func (a *MemoryServiceCatalogAdapter) RegisterService(ctx context.Context, service *catalogContracts.ServiceDescriptor) error {
	return observability.WithSpan(ctx, "memory.catalog.register", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		key := service.TenantID + ":" + service.ID
		if _, exists := a.store[key]; exists {
			return errors.NewConflictError("service already exists in catalog")
		}
		a.store[key] = service
		return nil
	}, attribute.String("db.system", "in-memory"), attribute.String("tenant_id", service.TenantID), attribute.String("service_id", service.ID))
}
