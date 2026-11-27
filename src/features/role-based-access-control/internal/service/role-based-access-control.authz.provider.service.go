/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC AUTHZ CAPABILITY PROVIDER (V.4 Invariant B, X-02)

 1. Role:
    - Implements the shared kernel authz.Authorizer interface for role-based-access-control.
    - Translates between kernel authz types and internal RBAC domain types at the capability boundary.
 2. Invariants:
    - Fail-closed when the backing RBACService is nil.
    - Zero mid-function inline comments.
*/
package service

import (
	"context"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/internal/contracts"
	"internal-developer-platform/src/shared/kernel"
	"internal-developer-platform/src/shared/kernel/capabilities/authz"
)

type RBACAuthorizerProvider struct {
	svc *RBACService
}

func NewRBACAuthorizerProvider(svc *RBACService) *RBACAuthorizerProvider {
	return &RBACAuthorizerProvider{svc: svc}
}

func (p *RBACAuthorizerProvider) Check(ctx context.Context, req authz.CheckRequest) (authz.Decision, *kernel.PortError) {
	if p.svc == nil {
		return authz.DecisionDeny, &kernel.PortError{
			Kind:      kernel.PortErrorUnavailable,
			Message:   "rbac authorizer unavailable",
			Retryable: true,
		}
	}
	roles := make([]rbacContracts.RoleName, 0, len(req.Principal.Roles))
	for _, r := range req.Principal.Roles {
		roles = append(roles, rbacContracts.RoleName(r))
	}
	evalReq := rbacContracts.EvaluationRequest{
		Subject: rbacContracts.Subject{
			ID:       req.Principal.Subject,
			TenantID: req.Principal.TenantID,
			Roles:    roles,
		},
		TargetTenantID: req.TenantID,
		Resource:       rbacContracts.Resource(req.Resource),
		Action:         rbacContracts.Action(req.Action),
	}
	res, err := p.svc.AuthorizeWithContext(ctx, evalReq)
	if err != nil {
		return authz.DecisionDeny, &kernel.PortError{
			Kind:    kernel.PortErrorRejected,
			Message: err.Error(),
			Cause:   err,
		}
	}
	if res.ElevatedPermit {
		return authz.DecisionElevatedPermit, nil
	}
	return authz.DecisionPermit, nil
}

func (p *RBACAuthorizerProvider) AuthenticateAndAuthorizeHTTP(ctx context.Context, rawToken string, req authz.RouteCheckRequest) (*authz.SubjectContext, error) {
	if p.svc == nil {
		return nil, &kernel.PortError{Kind: kernel.PortErrorUnavailable, Message: "rbac service unavailable", Retryable: true}
	}
	var bg *rbacContracts.BreakGlassContext
	if req.BreakGlassTicketID != "" {
		bg = &rbacContracts.BreakGlassContext{
			TicketID:            req.BreakGlassTicketID,
			TicketApproved:      req.BreakGlassApproved,
			Tier0Prod:           req.BreakGlassTier0Prod,
			DualApprovalGranted: req.BreakGlassDualAuth,
			TTLSeconds:          req.BreakGlassTTLSeconds,
		}
	}
	evalReq := rbacContracts.EvaluationRequest{
		TargetTenantID:         req.TargetTenantID,
		Resource:               rbacContracts.Resource(req.Resource),
		Action:                 rbacContracts.Action(req.Action),
		TargetEnvironment:      req.TargetEnvironment,
		ResourceClassification: req.ResourceClassification,
		ChangeFreezeActive:     req.ChangeFreezeActive,
		RequireStepUpMFA:       req.RequireStepUpMFA,
		GlobalAuthorityDown:    req.GlobalAuthorityDown,
		PolicySnapshotAgeSec:   req.PolicySnapshotAgeSec,
		BreakGlass:             bg,
	}
	sub, err := p.svc.AuthenticateAndAuthorizeWithContext(ctx, rawToken, evalReq)
	if err != nil {
		return nil, err
	}
	return subjectToAuthzContext(sub), nil
}

func (p *RBACAuthorizerProvider) VerifyImpersonation(ctx context.Context, callerMTLS bool, hasImpersonateGrant bool, targetProtectedAdmin bool) error {
	if p.svc == nil {
		return &kernel.PortError{Kind: kernel.PortErrorUnavailable, Message: "rbac service unavailable", Retryable: true}
	}
	return p.svc.VerifyImpersonation(ctx, callerMTLS, hasImpersonateGrant, targetProtectedAdmin)
}

func (p *RBACAuthorizerProvider) VerifyDelegationScope(ctx context.Context, isTenantDelegation bool, tokenTenantID string, targetTenantID string) error {
	if p.svc == nil {
		return &kernel.PortError{Kind: kernel.PortErrorUnavailable, Message: "rbac service unavailable", Retryable: true}
	}
	return p.svc.VerifyDelegationScope(ctx, isTenantDelegation, tokenTenantID, targetTenantID)
}

func subjectToAuthzContext(sub *rbacContracts.Subject) *authz.SubjectContext {
	if sub == nil {
		return nil
	}
	roles := make([]string, 0, len(sub.Roles))
	for _, r := range sub.Roles {
		roles = append(roles, string(r))
	}
	perms := make([]authz.Permission, 0, len(sub.Permissions))
	for _, p := range sub.Permissions {
		perms = append(perms, authz.Permission{
			Resource: authz.Resource(p.Resource),
			Action:   authz.Action(p.Action),
		})
	}
	return &authz.SubjectContext{
		ID:               sub.ID,
		TenantID:         sub.TenantID,
		Roles:            roles,
		GroupMemberships: sub.GroupMemberships,
		ClientType:       sub.ClientType,
		Permissions:      perms,
		MFAVerified:      sub.MFAVerified,
		Classification:   sub.Classification,
		Issuer:           sub.Issuer,
		ExpiresAt:        sub.ExpiresAt,
		JTI:              sub.JTI,
	}
}
