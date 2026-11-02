package adapters

import (
	"context"

	healthContracts "internal-developer-platform/src/features/health-checks/contracts"
)

type MemoryHealthCheckerAdapter struct{}

func NewMemoryHealthCheckerAdapter() *MemoryHealthCheckerAdapter {
	return &MemoryHealthCheckerAdapter{}
}

func (a *MemoryHealthCheckerAdapter) GetDatabaseHealth(ctx context.Context) healthContracts.ComponentHealth {
	return healthContracts.ComponentHealth{
		Component: "postgresql",
		Status:    healthContracts.StatusHealthy,
		Message:   "database connection active",
	}
}

func (a *MemoryHealthCheckerAdapter) GetCacheHealth(ctx context.Context) healthContracts.ComponentHealth {
	return healthContracts.ComponentHealth{
		Component: "redis",
		Status:    healthContracts.StatusHealthy,
		Message:   "cache ping ok",
	}
}
