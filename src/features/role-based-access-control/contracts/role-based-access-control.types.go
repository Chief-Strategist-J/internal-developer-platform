package rbac

import "time"

type RoleName string
type Action string
type Resource string

const (
	ActionRead   Action = "read"
	ActionWrite  Action = "write"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
	ActionAll    Action = "*"

	ResourceAll            Resource = "*"
	ResourceAuth           Resource = "auth"
	ResourceServiceCatalog Resource = "service-catalog"
	ResourceInfrastructure Resource = "infrastructure"
	ResourceSecrets        Resource = "secrets"

	RolePlatformAdmin RoleName = "PlatformAdmin"
	RoleDeveloper     RoleName = "Developer"
	RoleViewer        RoleName = "Viewer"
)

type Permission struct {
	Resource Resource `json:"resource"`
	Action   Action   `json:"action"`
}

type Role struct {
	Name        RoleName     `json:"name"`
	Permissions []Permission `json:"permissions"`
}

type Subject struct {
	ID          string     `json:"id"`
	TenantID    string     `json:"tenantId"`
	Roles       []RoleName `json:"roles"`
	Permissions []Permission `json:"permissions,omitempty"`
	Issuer      string     `json:"issuer"`
	ExpiresAt   time.Time  `json:"expiresAt"`
	JTI         string     `json:"jti,omitempty"`
}

type EvaluationRequest struct {
	Subject        Subject  `json:"subject"`
	TargetTenantID string   `json:"targetTenantId"`
	Resource       Resource `json:"resource"`
	Action         Action   `json:"action"`
}

type EvaluationResult struct {
	Granted bool   `json:"granted"`
	Reason  string `json:"reason"`
}
