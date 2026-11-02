package rbac

type IssueTokenRequest struct {
	SubjectID   string       `json:"subjectId"`
	TenantID    string       `json:"tenantId"`
	Roles       []RoleName   `json:"roles"`
	Permissions []Permission `json:"permissions,omitempty"`
	TTLSeconds  int64        `json:"ttlSeconds,omitempty"`
}

type IssueTokenResponse struct {
	AccessToken string   `json:"accessToken"`
	TokenType   string   `json:"tokenType"`
	ExpiresIn   int64    `json:"expiresIn"`
	SubjectID   string   `json:"subjectId"`
	TenantID    string   `json:"tenantId"`
	Roles       []RoleName `json:"roles"`
}

type RevokeSessionRequest struct {
	SubjectID  string `json:"subjectId,omitempty"`
	JTI        string `json:"jti,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type EvaluatePolicyRequest struct {
	TargetTenantID string   `json:"targetTenantId"`
	Resource       Resource `json:"resource"`
	Action         Action   `json:"action"`
}

type CallerIdentityResponse struct {
	SubjectID   string       `json:"subjectId"`
	TenantID    string       `json:"tenantId"`
	Roles       []RoleName   `json:"roles"`
	Permissions []Permission `json:"permissions,omitempty"`
	Issuer      string       `json:"issuer"`
	ExpiresAt   string       `json:"expiresAt"`
}
