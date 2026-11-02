package healthchecks

import (
	"context"

	healthContracts "internal-developer-platform/src/features/health-checks/contracts"
)

type HealthCheckPort interface {
	CheckHealth(ctx context.Context) healthContracts.ComponentHealth
}

type HealthCheckerRepositoryPort interface {
	GetDatabaseHealth(ctx context.Context) healthContracts.ComponentHealth
	GetCacheHealth(ctx context.Context) healthContracts.ComponentHealth
}
