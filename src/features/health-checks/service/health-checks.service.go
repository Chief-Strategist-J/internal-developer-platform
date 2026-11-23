// Package service provides domain health evaluation logic.
//
// Architectural Blueprint:
// - Step 06 Service: Evaluates overall platform health and subsystem readiness.
// - Spawns Tier-3 domain spans via observability.RunTraced.
// - 100% comment-free function bodies.
package service

import (
	"context"
	"time"

	healthContracts "internal-developer-platform/src/features/health-checks/contracts"
	healthPorts "internal-developer-platform/src/features/health-checks/infrastructure/ports"
	"internal-developer-platform/src/infra/observability"
)

type HealthCheckService struct {
	repo healthPorts.HealthCheckerRepositoryPort
}

func NewHealthCheckService(r healthPorts.HealthCheckerRepositoryPort) *HealthCheckService {
	return &HealthCheckService{repo: r}
}

func (s *HealthCheckService) EvaluateSystemHealth(ctx context.Context) healthContracts.SystemHealthReport {
	res, _ := observability.RunTraced(ctx, "health.service.evaluate_system_health", func(ctx context.Context) (healthContracts.SystemHealthReport, error) {
		dbHealth := s.repo.GetDatabaseHealth(ctx)
		cacheHealth := s.repo.GetCacheHealth(ctx)

		overall := healthContracts.StatusHealthy
		if dbHealth.Status == healthContracts.StatusUnhealthy || cacheHealth.Status == healthContracts.StatusUnhealthy {
			overall = healthContracts.StatusUnhealthy
		} else if dbHealth.Status == healthContracts.StatusDegraded || cacheHealth.Status == healthContracts.StatusDegraded {
			overall = healthContracts.StatusDegraded
		}

		return healthContracts.SystemHealthReport{
			Status:    overall,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Components: []healthContracts.ComponentHealth{
				dbHealth,
				cacheHealth,
			},
		}, nil
	})
	return res
}

func (s *HealthCheckService) CheckLiveness(ctx context.Context) bool {
	res, _ := observability.RunTraced(ctx, "health.service.check_liveness", func(ctx context.Context) (bool, error) {
		return true, nil
	})
	return res
}

func (s *HealthCheckService) CheckReadiness(ctx context.Context) bool {
	res, _ := observability.RunTraced(ctx, "health.service.check_readiness", func(ctx context.Context) (bool, error) {
		dbHealth := s.repo.GetDatabaseHealth(ctx)
		return dbHealth.Status != healthContracts.StatusUnhealthy, nil
	})
	return res
}
