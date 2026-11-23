// Package adapters provides outbound infrastructure adapters for Kubernetes Workload Provisioning.
//
// Architectural Blueprint:
// - Step 11 Adapter: In-memory repository adapter fulfilling WorkloadStorePort for tests and lightweight local regimes.
// - Thread-safe synchronization via sync.RWMutex.
// - 100% comment-free function bodies.
package adapters

import (
	"context"
	"sync"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/shared/errors"
)

type MemoryWorkloadRepositoryAdapter struct {
	mu      sync.RWMutex
	records map[string]contracts.WorkloadProvisionRecord
	byKey   map[string]string
}

func NewMemoryWorkloadRepositoryAdapter() *MemoryWorkloadRepositoryAdapter {
	return &MemoryWorkloadRepositoryAdapter{
		records: make(map[string]contracts.WorkloadProvisionRecord),
		byKey:   make(map[string]string),
	}
}

func (a *MemoryWorkloadRepositoryAdapter) Save(ctx context.Context, record contracts.WorkloadProvisionRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	lookupKey := record.TenantID + ":" + record.IdempotencyKey
	if _, exists := a.byKey[lookupKey]; exists {
		return errors.NewConflictError("idempotency key already exists for tenant")
	}

	primaryKey := record.TenantID + ":" + record.ID
	if _, exists := a.records[primaryKey]; exists {
		return errors.NewConflictError("workload provision record already exists")
	}

	a.records[primaryKey] = record
	a.byKey[lookupKey] = primaryKey
	return nil
}

func (a *MemoryWorkloadRepositoryAdapter) GetByID(ctx context.Context, tenantID string, id string) (*contracts.WorkloadProvisionRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	key := tenantID + ":" + id
	record, exists := a.records[key]
	if !exists {
		return nil, errors.NewNotFoundError("workload provision record not found")
	}
	return &record, nil
}

func (a *MemoryWorkloadRepositoryAdapter) GetByIdempotencyKey(ctx context.Context, tenantID string, key string) (*contracts.WorkloadProvisionRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	lookupKey := tenantID + ":" + key
	primaryKey, exists := a.byKey[lookupKey]
	if !exists {
		return nil, nil
	}

	record, found := a.records[primaryKey]
	if !found {
		return nil, nil
	}
	return &record, nil
}

func (a *MemoryWorkloadRepositoryAdapter) List(ctx context.Context, filter contracts.FilterCriteria) ([]contracts.WorkloadProvisionRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	results := make([]contracts.WorkloadProvisionRecord, 0)
	for _, rec := range a.records {
		if rec.TenantID == filter.TenantID {
			if filter.ServiceID != "" && rec.ServiceID != filter.ServiceID {
				continue
			}
			if filter.Status != "" && rec.Status != filter.Status {
				continue
			}
			results = append(results, rec)
		}
	}
	return results, nil
}

func (a *MemoryWorkloadRepositoryAdapter) Update(ctx context.Context, record contracts.WorkloadProvisionRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := record.TenantID + ":" + record.ID
	existing, exists := a.records[key]
	if !exists {
		return errors.NewNotFoundError("workload provision record not found")
	}

	if existing.Version != record.Version-1 && record.Version > 1 {
		return errors.NewConflictError("optimistic concurrency conflict on update")
	}

	a.records[key] = record
	return nil
}

