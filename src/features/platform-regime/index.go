/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PLATFORM REGIME INDEX

1. Package Entrypoint:
   - Exposes clean constructors and types for the platform-regime feature.
   - Zero comments inside method bodies.
*/
package platformregime

import (
	"database/sql"

	"internal-developer-platform/src/features/platform-regime/contracts"
	"internal-developer-platform/src/features/platform-regime/infrastructure/adapters"
	"internal-developer-platform/src/features/platform-regime/infrastructure/ports"
	"internal-developer-platform/src/features/platform-regime/service"
)

type PlatformRegimeService = service.PlatformRegimeService
type PlatformRegimeRepositoryPort = ports.PlatformRegimeRepositoryPort

func NewService(repo ports.PlatformRegimeRepositoryPort, baseline contracts.PlatformRegimePolicyRecord) *PlatformRegimeService {
	return service.NewPlatformRegimeService(repo, baseline)
}

func NewMemoryRepository() ports.PlatformRegimeRepositoryPort {
	return adapters.NewMemoryPlatformRegimeRepositoryAdapter()
}

func NewPostgresRepository(db *sql.DB) ports.PlatformRegimeRepositoryPort {
	return adapters.NewPostgresPlatformRegimeRepositoryAdapter(db)
}
