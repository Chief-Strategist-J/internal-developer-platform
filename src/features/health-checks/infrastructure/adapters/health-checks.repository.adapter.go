// Package adapters provides health checker repository adapters.
//
// Architectural Blueprint:
// - Step 11 Adapter: In-memory health checker adapter probing database and cache health.
// - Spawns Tier-4 infrastructure spans via observability.RunTraced.
// - 100% comment-free function bodies.
package adapters

import (
	"context"

	"go.opentelemetry.io/otel/attribute"

	healthContracts "internal-developer-platform/src/features/health-checks/contracts"
	"internal-developer-platform/src/infra/observability"
)

type MemoryHealthCheckerAdapter struct{}

func NewMemoryHealthCheckerAdapter() *MemoryHealthCheckerAdapter {
	return &MemoryHealthCheckerAdapter{}
}

func (a *MemoryHealthCheckerAdapter) GetDatabaseHealth(ctx context.Context) healthContracts.ComponentHealth {
	res, _ := observability.RunTraced(ctx, "health.probe.database", func(ctx context.Context) (healthContracts.ComponentHealth, error) {
		return healthContracts.ComponentHealth{
			Component: "postgresql",
			Status:    healthContracts.StatusHealthy,
			Message:   "database connection active",
		}, nil
	}, attribute.String("component", "postgresql"))
	return res
}

func (a *MemoryHealthCheckerAdapter) GetCacheHealth(ctx context.Context) healthContracts.ComponentHealth {
	res, _ := observability.RunTraced(ctx, "health.probe.cache", func(ctx context.Context) (healthContracts.ComponentHealth, error) {
		return healthContracts.ComponentHealth{
			Component: "redis",
			Status:    healthContracts.StatusHealthy,
			Message:   "cache ping ok",
		}, nil
	}, attribute.String("component", "redis"))
	return res
}
