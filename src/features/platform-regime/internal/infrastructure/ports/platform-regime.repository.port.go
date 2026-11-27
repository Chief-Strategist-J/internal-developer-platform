/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PLATFORM REGIME REPOSITORY PORT

1. Interface Definition:
  - Save: Persists a PlatformRegimePolicyRecord in the backing store.
  - GetActive: Retrieves the latest ACTIVE policy for a tenant.
  - GetByID: Retrieves a specific policy by tenant ID and policy ID.
  - Zero comments inside method bodies.
*/
package ports

import (
	"context"

	"internal-developer-platform/src/features/platform-regime/internal/contracts"
)

type PlatformRegimeRepositoryPort interface {
	Save(ctx context.Context, record contracts.PlatformRegimePolicyRecord) error
	GetActive(ctx context.Context, tenantID string) (*contracts.PlatformRegimePolicyRecord, error)
	GetByID(ctx context.Context, tenantID string, id string) (*contracts.PlatformRegimePolicyRecord, error)
}
