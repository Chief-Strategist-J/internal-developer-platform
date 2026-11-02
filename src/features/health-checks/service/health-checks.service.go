package service

import (
	"context"
	"time"

	healthContracts "internal-developer-platform/src/features/health-checks/contracts"
	healthPorts "internal-developer-platform/src/features/health-checks/infrastructure/ports"
)

type HealthCheckService struct {
	repo healthPorts.HealthCheckerRepositoryPort
}

func NewHealthCheckService(r healthPorts.HealthCheckerRepositoryPort) *HealthCheckService {
	return &HealthCheckService{repo: r}
}

func (s *HealthCheckService) EvaluateSystemHealth(ctx context.Context) healthContracts.SystemHealthReport {
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
	}
}

func (s *HealthCheckService) CheckLiveness(ctx context.Context) bool {
	return true
}

func (s *HealthCheckService) CheckReadiness(ctx context.Context) bool {
	dbHealth := s.repo.GetDatabaseHealth(ctx)
	return dbHealth.Status != healthContracts.StatusUnhealthy
}
