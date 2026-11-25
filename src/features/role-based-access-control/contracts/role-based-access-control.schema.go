package rbac

import "time"

type IssueTokenRequest struct {
	SubjectID   string       `json:"subjectId"`
	TenantID    string       `json:"tenantId"`
	Roles       []RoleName   `json:"roles"`
	Permissions []Permission `json:"permissions,omitempty"`
	TTLSeconds  int64        `json:"ttlSeconds,omitempty"`
}

type IssueTokenResponse struct {
	AccessToken string     `json:"accessToken"`
	TokenType   string     `json:"tokenType"`
	ExpiresIn   int64      `json:"expiresIn"`
	SubjectID   string     `json:"subjectId"`
	TenantID    string     `json:"tenantId"`
	Roles       []RoleName `json:"roles"`
}

type RevokeSessionRequest struct {
	SubjectID string `json:"subjectId,omitempty"`
	JTI       string `json:"jti,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type EvaluatePolicyRequest struct {
	TargetTenantID         string   `json:"targetTenantId"`
	Resource               Resource `json:"resource"`
	Action                 Action   `json:"action"`
	TargetEnvironment      string   `json:"targetEnvironment,omitempty"`
	ChangeFreezeActive     bool     `json:"changeFreezeActive,omitempty"`
	RequireStepUpMFA       bool     `json:"requireStepUpMfa,omitempty"`
	ResourceClassification string   `json:"resourceClassification,omitempty"`
}

type ValidateRoleHierarchyRequest struct {
	Edges map[string][]string `json:"edges"`
}

type ValidateRoleHierarchyResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

type CallerIdentityResponse struct {
	SubjectID   string       `json:"subjectId"`
	TenantID    string       `json:"tenantId"`
	Roles       []RoleName   `json:"roles"`
	Permissions []Permission `json:"permissions,omitempty"`
	Issuer      string       `json:"issuer"`
	ExpiresAt   string       `json:"expiresAt"`
}

type AccessGrantRequest struct {
	ID              string `json:"id"`
	TenantID        string `json:"tenantId"`
	SubjectID       string `json:"subjectId"`
	TargetNamespace string `json:"targetNamespace"`
	Role            string `json:"role"`
	Reason          string `json:"reason"`
	DurationSeconds int    `json:"durationSeconds"`
	ApproverRole    string `json:"approverRole"`
	IdempotencyKey  string `json:"idempotencyKey"`
}

type AccessGrantRecord struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenantId"`
	SubjectID       string    `json:"subjectId"`
	TargetNamespace string    `json:"targetNamespace"`
	Role            string    `json:"role"`
	Reason          string    `json:"reason"`
	DurationSeconds int       `json:"durationSeconds"`
	ApprovedBy      string    `json:"approvedBy"`
	Status          string    `json:"status"`
	ExpiresAt       time.Time `json:"expiresAt"`
	IdempotencyKey  string    `json:"idempotencyKey"`
	Version         int64     `json:"version"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type RevokeGrantRequest struct {
	GrantID  string `json:"grantId"`
	TenantID string `json:"tenantId"`
	Reason   string `json:"reason,omitempty"`
}

type UserBindingRecord struct {
	SubjectID string    `json:"subjectId"`
	TenantID  string    `json:"tenantId"`
	RoleName  RoleName  `json:"roleName"`
	CreatedAt time.Time `json:"createdAt"`
}

type BindUserRoleRequest struct {
	SubjectID string   `json:"subjectId"`
	TenantID  string   `json:"tenantId"`
	RoleName  RoleName `json:"roleName"`
}

type UnbindUserRoleRequest struct {
	SubjectID string   `json:"subjectId"`
	TenantID  string   `json:"tenantId"`
	RoleName  RoleName `json:"roleName"`
}

type UserBindingsResponse struct {
	SubjectID string     `json:"subjectId"`
	TenantID  string     `json:"tenantId"`
	Roles     []RoleName `json:"roles"`
}
