// Package authz declares the kernel Authorizer capability contract (Part IV.2, X-02).
//
// Blueprint:
//  1. This file contains interfaces and pure kernel value types only — no IO or implementations.
//  2. role-based-access-control provides this capability; infrastructure middleware and consumers depend only on this contract.
//  3. Fail-closed: when the provider is unavailable, return PortError{Kind: PortErrorUnavailable}.
//
// Invariants:
//   - Two or more features consume this capability before promotion to the kernel (K-01).
//   - Zero imports of any src/features/* package.
package authz

import (
	"context"
	"time"

	"internal-developer-platform/src/shared/kernel"
)

type Resource string
type Action string
type Decision string

const (
	DecisionPermit         Decision = "PERMIT"
	DecisionDeny           Decision = "DENY"
	DecisionElevatedPermit Decision = "ELEVATED_PERMIT"

	ResourceAll            Resource = "*"
	ResourceAuth           Resource = "auth"
	ResourceServiceCatalog Resource = "service-catalog"
	ResourceInfrastructure Resource = "infrastructure"
	ResourceSecrets        Resource = "secrets"
	ResourceAuditLogs      Resource = "audit-logs"
	ResourceCustomerPII    Resource = "customer-pii"

	ActionAll           Action = "*"
	ActionRead          Action = "read"
	ActionWrite         Action = "write"
	ActionUpdate        Action = "update"
	ActionDelete        Action = "delete"
	ActionScale         Action = "scale"
	ActionExecute       Action = "execute"
	ActionManageSecrets Action = "manage-secrets"
)

type Permission struct {
	Resource Resource `json:"resource"`
	Action   Action   `json:"action"`
}

type SubjectContext struct {
	ID               string       `json:"id"`
	TenantID         string       `json:"tenantId"`
	Roles            []string     `json:"roles"`
	GroupMemberships []string     `json:"groupMemberships,omitempty"`
	ClientType       string       `json:"clientType,omitempty"`
	Permissions      []Permission `json:"permissions,omitempty"`
	MFAVerified      bool         `json:"mfaVerified,omitempty"`
	Classification   string       `json:"classification,omitempty"`
	Issuer           string       `json:"issuer"`
	ExpiresAt        time.Time    `json:"expiresAt"`
	JTI              string       `json:"jti,omitempty"`
}

type CheckRequest struct {
	Principal kernel.Principal
	Action    string
	Resource  string
	TenantID  string
}

type RouteCheckRequest struct {
	TargetTenantID         string
	Resource               Resource
	Action                 Action
	TargetEnvironment      string
	ResourceClassification string
	ChangeFreezeActive     bool
	RequireStepUpMFA       bool
	GlobalAuthorityDown    bool
	PolicySnapshotAgeSec   int64
	BreakGlassTicketID     string
	BreakGlassApproved     bool
	BreakGlassTier0Prod    bool
	BreakGlassDualAuth     bool
	BreakGlassTTLSeconds   int
}

type Authorizer interface {
	Check(ctx context.Context, req CheckRequest) (Decision, *kernel.PortError)
	AuthenticateAndAuthorizeHTTP(ctx context.Context, rawToken string, req RouteCheckRequest) (*SubjectContext, error)
	VerifyImpersonation(ctx context.Context, callerMTLS bool, hasImpersonateGrant bool, targetProtectedAdmin bool) error
	VerifyDelegationScope(ctx context.Context, isTenantDelegation bool, tokenTenantID string, targetTenantID string) error
}
