package servicecatalog

type ServiceStatus string

const (
	ServiceStatusDraft      ServiceStatus = "DRAFT"
	ServiceStatusActive     ServiceStatus = "ACTIVE"
	ServiceStatusDeprecated ServiceStatus = "DEPRECATED"
	ServiceStatusRetired    ServiceStatus = "RETIRED"
)

type ServiceDescriptor struct {
	ID          string        `json:"id"`
	TenantID    string        `json:"tenantId"`
	Name        string        `json:"name"`
	Owner       string        `json:"owner"`
	Lifecycle   string        `json:"lifecycle"`
	Status      ServiceStatus `json:"status"`
	Version     int64         `json:"version"`
	CreatedAt   string        `json:"createdAt"`
	UpdatedAt   string        `json:"updatedAt"`
}

type RegisterServiceInput struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenantId"`
	Name      string `json:"name"`
	Owner     string `json:"owner"`
	Lifecycle string `json:"lifecycle"`
}
