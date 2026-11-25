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
	ResourceAuditLogs      Resource = "audit-logs"
	ResourceCustomerPII    Resource = "customer-pii"

	RolePlatformAdmin RoleName = "PlatformAdmin"
	RoleDeveloper     RoleName = "Developer"
	RoleViewer        RoleName = "Viewer"

	EnvDevelopment string = "DEV"
	EnvStaging     string = "STAGING"
	EnvProduction  string = "PRODUCTION"
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
	ID              string       `json:"id"`
	TenantID        string       `json:"tenantId"`
	Roles           []RoleName   `json:"roles"`
	Permissions     []Permission `json:"permissions,omitempty"`
	DenyPermissions []Permission `json:"denyPermissions,omitempty"`
	MFAVerified     bool         `json:"mfaVerified,omitempty"`
	Classification  string       `json:"classification,omitempty"`
	Issuer          string       `json:"issuer"`
	ExpiresAt       time.Time    `json:"expiresAt"`
	JTI             string       `json:"jti,omitempty"`
}

type BreakGlassContext struct {
	TicketID            string `json:"ticketId"`
	TicketApproved      bool   `json:"ticketApproved"`
	TTLSeconds          int    `json:"ttlSeconds"`
	Tier0Prod           bool   `json:"tier0Prod"`
	DualApprovalGranted bool   `json:"dualApprovalGranted"`
}

type EvaluationRequest struct {
	Subject                Subject            `json:"subject"`
	TargetTenantID         string             `json:"targetTenantId"`
	Resource               Resource           `json:"resource"`
	Action                 Action             `json:"action"`
	TargetEnvironment      string             `json:"targetEnvironment,omitempty"`
	ChangeFreezeActive     bool               `json:"changeFreezeActive,omitempty"`
	RequireStepUpMFA       bool               `json:"requireStepUpMfa,omitempty"`
	ResourceClassification string             `json:"resourceClassification,omitempty"`
	BreakGlass             *BreakGlassContext `json:"breakGlass,omitempty"`
}

type EvaluationResult struct {
	Granted        bool   `json:"granted"`
	ElevatedPermit bool   `json:"elevatedPermit,omitempty"`
	Reason         string `json:"reason"`
	ViolationCode  string `json:"violationCode,omitempty"`
}

type RBACAuditEvent struct {
	ID            string    `json:"id"`
	TenantID      string    `json:"tenantId"`
	SubjectID     string    `json:"subjectId"`
	Resource      Resource  `json:"resource"`
	Action        Action    `json:"action"`
	Decision      string    `json:"decision"`
	Reason        string    `json:"reason"`
	ViolationCode string    `json:"violationCode,omitempty"`
	OccurredAt    time.Time `json:"occurredAt"`
}
