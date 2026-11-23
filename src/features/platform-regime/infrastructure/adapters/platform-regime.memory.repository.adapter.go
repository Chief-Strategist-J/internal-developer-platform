/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PLATFORM REGIME MEMORY REPOSITORY ADAPTER

1. In-Memory Store Adapter:
   - Thread-safe in-memory adapter for hermetic testing and runtime fallback.
   - Enforces Tier-4 infrastructure spans for all operations.
   - Zero comments inside method bodies.
*/
package adapters

import (
	"context"
	"sync"

	"internal-developer-platform/src/features/platform-regime/contracts"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type MemoryPlatformRegimeRepositoryAdapter struct {
	mu      sync.RWMutex
	records map[string]contracts.PlatformRegimePolicyRecord
}

func NewMemoryPlatformRegimeRepositoryAdapter() *MemoryPlatformRegimeRepositoryAdapter {
	return &MemoryPlatformRegimeRepositoryAdapter{
		records: make(map[string]contracts.PlatformRegimePolicyRecord),
	}
}

func (m *MemoryPlatformRegimeRepositoryAdapter) Save(ctx context.Context, record contracts.PlatformRegimePolicyRecord) error {
	return observability.WithSpan(ctx, "memory.regime.save", func(ctx context.Context) error {
		m.mu.Lock()
		defer m.mu.Unlock()

		if record.Status == contracts.RegimeStatusActive {
			for k, v := range m.records {
				if v.TenantID == record.TenantID && v.Status == contracts.RegimeStatusActive {
					v.Status = contracts.RegimeStatusSuperseded
					m.records[k] = v
				}
			}
		}

		m.records[record.ID] = record
		return nil
	})
}

func (m *MemoryPlatformRegimeRepositoryAdapter) GetActive(ctx context.Context, tenantID string) (*contracts.PlatformRegimePolicyRecord, error) {
	return observability.RunTraced(ctx, "memory.regime.get_active", func(ctx context.Context) (*contracts.PlatformRegimePolicyRecord, error) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		var active *contracts.PlatformRegimePolicyRecord
		for _, v := range m.records {
			if v.TenantID == tenantID && v.Status == contracts.RegimeStatusActive {
				rec := v
				active = &rec
				break
			}
		}

		if active == nil {
			return nil, errors.NewNotFoundError("active platform regime policy not found")
		}
		return active, nil
	})
}

func (m *MemoryPlatformRegimeRepositoryAdapter) GetByID(ctx context.Context, tenantID string, id string) (*contracts.PlatformRegimePolicyRecord, error) {
	return observability.RunTraced(ctx, "memory.regime.get_by_id", func(ctx context.Context) (*contracts.PlatformRegimePolicyRecord, error) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		rec, exists := m.records[id]
		if !exists || rec.TenantID != tenantID {
			return nil, errors.NewNotFoundError("platform regime policy not found")
		}
		return &rec, nil
	})
}
