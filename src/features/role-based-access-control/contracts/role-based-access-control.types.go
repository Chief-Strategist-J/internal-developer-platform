package rbac

import "time"

type RoleName string
type Action string
type Resource string

const (
	ActionRead          Action = "read"
	ActionWrite         Action = "write"
	ActionUpdate        Action = "update"
	ActionDelete        Action = "delete"
	ActionScale         Action = "scale"
	ActionExecute       Action = "execute"
	ActionManageSecrets Action = "manage-secrets"
	ActionAll           Action = "*"

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
	ID               string       `json:"id"`
	TenantID         string       `json:"tenantId"`
	Roles            []RoleName   `json:"roles"`
	GroupMemberships []string     `json:"groupMemberships,omitempty"`
	ClientType       string       `json:"clientType,omitempty"`
	Permissions          []Permission        `json:"permissions,omitempty"`
	DenyPermissions      []Permission        `json:"denyPermissions,omitempty"`
	FlattenedPermissions map[string]struct{} `json:"-"`
	MFAVerified      bool         `json:"mfaVerified,omitempty"`
	Classification   string       `json:"classification,omitempty"`
	Issuer           string       `json:"issuer"`
	ExpiresAt        time.Time    `json:"expiresAt"`
	JTI              string       `json:"jti,omitempty"`
}

type BreakGlassContext struct {
	TicketID            string `json:"ticketId"`
	TicketApproved      bool   `json:"ticketApproved"`
	TTLSeconds          int    `json:"ttlSeconds"`
	Tier0Prod           bool   `json:"tier0Prod"`
	DualApprovalGranted bool   `json:"dualApprovalGranted"`
}

type FederationHandshake struct {
	SourceTenantID string `json:"sourceTenantId"`
	TargetTenantID string `json:"targetTenantId"`
	Signature      string `json:"signature"`
	ExpiresAtUnix  int64  `json:"expiresAtUnix"`
}

type EvaluationRequest struct {
	Subject                 Subject                    `json:"subject"`
	TargetTenantID          string                     `json:"targetTenantId"`
	TargetNamespace         string                     `json:"targetNamespace,omitempty"`
	Resource                Resource                   `json:"resource"`
	Action                  Action                     `json:"action"`
	TargetEnvironment       string                     `json:"targetEnvironment,omitempty"`
	ChangeFreezeActive      bool                       `json:"changeFreezeActive,omitempty"`
	RequireStepUpMFA        bool                       `json:"requireStepUpMfa,omitempty"`
	ResourceClassification  string                     `json:"resourceClassification,omitempty"`
	CurrentQuotaUsage       int                        `json:"currentQuotaUsage,omitempty"`
	MaxTenantQuota          int                        `json:"maxTenantQuota,omitempty"`
	AllowedStartHourUTC     int                        `json:"allowedStartHourUtc,omitempty"`
	AllowedEndHourUTC       int                        `json:"allowedEndHourUtc,omitempty"`
	BreakGlass              *BreakGlassContext         `json:"breakGlass,omitempty"`
	CrossTenantFederation   *FederationHandshake       `json:"crossTenantFederation,omitempty"`
	FederationHMACSecret    string                     `json:"-"`
	ResolvedRolePermissions map[RoleName][]Permission  `json:"-"`
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

type TaskExecutionLeaseRequest struct {
	TokenExpired          bool `json:"tokenExpired"`
	IsIdempotentBatchStep bool `json:"isIdempotentBatchStep"`
	HasSignedTaskLease    bool `json:"hasSignedTaskLease"`
	LeaseExpired          bool `json:"leaseExpired"`
	RefreshSucceeded      bool `json:"refreshSucceeded"`
}

type TaskExecutionLeaseResult struct {
	Granted       bool   `json:"granted"`
	Directive     string `json:"directive"`
	Reason        string `json:"reason"`
	ViolationCode string `json:"violationCode,omitempty"`
}

type AsyncJobRevocationResult struct {
	Granted       bool   `json:"granted"`
	Directive     string `json:"directive"`
	Reason        string `json:"reason"`
	ViolationCode string `json:"violationCode,omitempty"`
}

type RegionalSplitBrainResult struct {
	Granted       bool   `json:"granted"`
	Degraded      bool   `json:"degraded,omitempty"`
	HTTPStatus    int    `json:"httpStatus"`
	WarningHeader string `json:"warningHeader,omitempty"`
	Reason        string `json:"reason"`
	ViolationCode string `json:"violationCode,omitempty"`
}

type StreamingHeartbeatResult struct {
	Terminate       bool          `json:"terminate"`
	SessionDuration time.Duration `json:"sessionDuration"`
	Reason          string        `json:"reason"`
}

type AccessGrantPage struct {
	Items      []AccessGrantRecord `json:"items"`
	NextCursor string              `json:"nextCursor,omitempty"`
	HasMore    bool                `json:"hasMore"`
}

type SidecarPDPEngine string

const (
	SidecarEngineOPA   SidecarPDPEngine = "opa"
	SidecarEngineCedar SidecarPDPEngine = "cedar"
)

type SidecarPDPConfig struct {
	Engine     SidecarPDPEngine `json:"engine"`
	Endpoint   string           `json:"endpoint"`
	PolicyPath string           `json:"policyPath"`
	TimeoutMs  int              `json:"timeoutMs"`
}

type SidecarDecisionRequest struct {
	SubjectID   string            `json:"subjectId"`
	TenantID    string            `json:"tenantId"`
	Resource    Resource          `json:"resource"`
	Action      Action            `json:"action"`
	Environment string            `json:"environment"`
	Attributes  map[string]string `json:"attributes,omitempty"`
}

type SidecarDecisionResult struct {
	Allowed       bool             `json:"allowed"`
	Engine        SidecarPDPEngine `json:"engine"`
	Reason        string           `json:"reason"`
	ViolationCode string           `json:"violationCode,omitempty"`
}

type EBPFHookEvent struct {
	WorkloadID string `json:"workloadId"`
	Namespace  string `json:"namespace"`
	Syscall    string `json:"syscall"`
	TargetPath string `json:"targetPath"`
	UID        uint32 `json:"uid"`
}

type EBPFHookPolicy struct {
	AllowedSyscalls []string `json:"allowedSyscalls"`
	DeniedPaths     []string `json:"deniedPaths"`
	Enforce         bool     `json:"enforce"`
}

type PermissionUsageRecord struct {
	Role       RoleName  `json:"role"`
	Resource   Resource  `json:"resource"`
	Action     Action    `json:"action"`
	LastUsedAt time.Time `json:"lastUsedAt"`
	HitCount   int64     `json:"hitCount"`
}

type UnusedPermissionFinding struct {
	Role       RoleName  `json:"role"`
	Permission Permission `json:"permission"`
	LastUsedAt time.Time `json:"lastUsedAt,omitempty"`
	Reason     string    `json:"reason"`
}

type LeastPrivilegeReport struct {
	EvaluatedAt time.Time                 `json:"evaluatedAt"`
	TotalRoles  int                       `json:"totalRoles"`
	Findings    []UnusedPermissionFinding `json:"findings"`
}
