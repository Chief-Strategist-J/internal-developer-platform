package rbac

import (
	"time"

	"internal-developer-platform/src/shared/errors"
)

type IssueTokenRequest struct {
	SubjectID        string       `json:"subjectId"`
	TenantID         string       `json:"tenantId"`
	Roles            []RoleName   `json:"roles"`
	GroupMemberships []string     `json:"groupMemberships,omitempty"`
	ClientType       string       `json:"clientType,omitempty"`
	Permissions      []Permission `json:"permissions,omitempty"`
	MFAVerified      bool         `json:"mfaVerified,omitempty"`
	Classification   string       `json:"classification,omitempty"`
	TTLSeconds       int64        `json:"ttlSeconds,omitempty"`
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
	TargetTenantID         string             `json:"targetTenantId"`
	Resource               Resource           `json:"resource"`
	Action                 Action             `json:"action"`
	TargetEnvironment      string             `json:"targetEnvironment,omitempty"`
	ChangeFreezeActive     bool               `json:"changeFreezeActive,omitempty"`
	RequireStepUpMFA       bool               `json:"requireStepUpMfa,omitempty"`
	ResourceClassification string             `json:"resourceClassification,omitempty"`
	BreakGlass             *BreakGlassContext `json:"breakGlass,omitempty"`
}

type ValidateRoleHierarchyRequest struct {
	Edges map[string][]string `json:"edges"`
}

type ValidateRoleHierarchyResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

type CallerIdentityResponse struct {
	SubjectID        string       `json:"subjectId"`
	TenantID         string       `json:"tenantId"`
	Roles            []RoleName   `json:"roles"`
	GroupMemberships []string     `json:"groupMemberships,omitempty"`
	ClientType       string       `json:"clientType,omitempty"`
	Permissions      []Permission `json:"permissions,omitempty"`
	MFAVerified      bool         `json:"mfaVerified,omitempty"`
	Classification   string       `json:"classification,omitempty"`
	Issuer           string       `json:"issuer"`
	ExpiresAt        string       `json:"expiresAt"`
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

func SubjectFromIssueTokenAPI(req IssueTokenRequest, expiresAt time.Time) Subject {
	return Subject{
		ID:               req.SubjectID,
		TenantID:         req.TenantID,
		Roles:            req.Roles,
		GroupMemberships: req.GroupMemberships,
		ClientType:       req.ClientType,
		Permissions:      req.Permissions,
		MFAVerified:      req.MFAVerified,
		Classification:   req.Classification,
		ExpiresAt:        expiresAt,
	}
}

func EvaluationRequestFromAPI(sub Subject, req EvaluatePolicyRequest) EvaluationRequest {
	return EvaluationRequest{
		Subject:                sub,
		TargetTenantID:         req.TargetTenantID,
		Resource:               req.Resource,
		Action:                 req.Action,
		TargetEnvironment:      req.TargetEnvironment,
		ChangeFreezeActive:     req.ChangeFreezeActive,
		RequireStepUpMFA:       req.RequireStepUpMFA,
		ResourceClassification: req.ResourceClassification,
		BreakGlass:             req.BreakGlass,
	}
}

func CallerIdentityToAPI(sub Subject) CallerIdentityResponse {
	return CallerIdentityResponse{
		SubjectID:        sub.ID,
		TenantID:         sub.TenantID,
		Roles:            sub.Roles,
		GroupMemberships: sub.GroupMemberships,
		ClientType:       sub.ClientType,
		Permissions:      sub.Permissions,
		MFAVerified:      sub.MFAVerified,
		Classification:   sub.Classification,
		Issuer:           sub.Issuer,
		ExpiresAt:        sub.ExpiresAt.UTC().Format(time.RFC3339),
	}
}

func ValidateIssueTokenInput(req IssueTokenRequest) error {
	if req.SubjectID == "" || req.TenantID == "" {
		return errors.NewValidationError("subjectId and tenantId are required")
	}
	return nil
}

func ValidateRevokeSessionInput(req RevokeSessionRequest) error {
	if req.SubjectID == "" && req.JTI == "" {
		return errors.NewValidationError("either subjectId or jti must be provided")
	}
	return nil
}

func ValidateEvaluatePolicyInput(req EvaluatePolicyRequest) error {
	if req.Resource == "" || req.Action == "" {
		return errors.NewValidationError("resource and action are required")
	}
	return nil
}

func ValidateBindUserRoleInput(req BindUserRoleRequest) error {
	if req.SubjectID == "" || req.TenantID == "" || req.RoleName == "" {
		return errors.NewValidationError("subjectId, tenantId, and roleName are required")
	}
	return nil
}

func ValidateUnbindUserRoleInput(req UnbindUserRoleRequest) error {
	if req.SubjectID == "" || req.TenantID == "" || req.RoleName == "" {
		return errors.NewValidationError("subjectId, tenantId, and roleName are required")
	}
	return nil
}
