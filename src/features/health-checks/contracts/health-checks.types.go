package healthchecks

type HealthStatus string

const (
	StatusHealthy   HealthStatus = "HEALTHY"
	StatusDegraded  HealthStatus = "DEGRADED"
	StatusUnhealthy HealthStatus = "UNHEALTHY"
)

type ComponentHealth struct {
	Component string       `json:"component"`
	Status    HealthStatus `json:"status"`
	Message   string       `json:"message"`
}

type SystemHealthReport struct {
	Status     HealthStatus      `json:"status"`
	Timestamp  string            `json:"timestamp"`
	Components []ComponentHealth `json:"components"`
}
