package healthchecks

import (
	healthContracts "internal-developer-platform/src/features/health-checks/contracts"
	healthService "internal-developer-platform/src/features/health-checks/service"
)

type Service = healthService.HealthCheckService
type HealthReport = healthContracts.SystemHealthReport
type Status = healthContracts.HealthStatus
