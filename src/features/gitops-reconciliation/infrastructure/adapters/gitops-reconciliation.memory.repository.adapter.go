package adapters

import (
	"context"
	"sync"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/shared/errors"
)

type MemoryGitOpsRepositoryAdapter struct {
	mu      sync.RWMutex
	records map[string]contracts.ReconciliationRecord
	byKey   map[string]string
}

func NewMemoryGitOpsRepositoryAdapter() *MemoryGitOpsRepositoryAdapter {
	return &MemoryGitOpsRepositoryAdapter{
		records: make(map[string]contracts.ReconciliationRecord),
		byKey:   make(map[string]string),
	}
}

func (a *MemoryGitOpsRepositoryAdapter) Save(ctx context.Context, record contracts.ReconciliationRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	lookupKey := record.TenantID + ":" + record.IdempotencyKey
	if _, exists := a.byKey[lookupKey]; exists {
		return errors.NewConflictError("idempotency key already exists for tenant")
	}

	primaryKey := record.TenantID + ":" + record.ID
	if _, exists := a.records[primaryKey]; exists {
		return errors.NewConflictError("reconciliation record already exists")
	}

	a.records[primaryKey] = record
	a.byKey[lookupKey] = primaryKey
	return nil
}

func (a *MemoryGitOpsRepositoryAdapter) GetByID(ctx context.Context, tenantID string, id string) (*contracts.ReconciliationRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	key := tenantID + ":" + id
	record, exists := a.records[key]
	if !exists {
		return nil, errors.NewNotFoundError("reconciliation record not found")
	}
	return &record, nil
}

func (a *MemoryGitOpsRepositoryAdapter) GetByIdempotencyKey(ctx context.Context, tenantID string, key string) (*contracts.ReconciliationRecord, error) {
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

func (a *MemoryGitOpsRepositoryAdapter) List(ctx context.Context, filter contracts.FilterCriteria) ([]contracts.ReconciliationRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	results := make([]contracts.ReconciliationRecord, 0)
	for _, rec := range a.records {
		if rec.TenantID == filter.TenantID {
			if filter.ServiceID != "" && rec.ServiceID != filter.ServiceID {
				continue
			}
			if filter.SyncPhase != "" && rec.SyncPhase != filter.SyncPhase {
				continue
			}
			results = append(results, rec)
		}
	}
	return results, nil
}

func (a *MemoryGitOpsRepositoryAdapter) Update(ctx context.Context, record contracts.ReconciliationRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := record.TenantID + ":" + record.ID
	existing, exists := a.records[key]
	if !exists {
		return errors.NewNotFoundError("reconciliation record not found")
	}

	if existing.Version != record.Version-1 && record.Version > 1 {
		return errors.NewConflictError("optimistic concurrency conflict on update")
	}

	a.records[key] = record
	return nil
}

