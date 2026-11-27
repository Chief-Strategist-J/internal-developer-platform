/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC INGRESS MIDDLEWARE

 1. Ingress Execution (G-06, VI.1 #9):
    - Extracts Bearer JWT and ABAC / Break-Glass / Split-Brain HTTP headers.
    - Delegates authorization strictly through the kernel authz.Authorizer capability (X-02).
    - Stores verified *authz.SubjectContext on request context without importing feature internals (G-01).
    - Zero mid-function inline comments.
*/
package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"internal-developer-platform/src/shared/constants"
	sharedErrors "internal-developer-platform/src/shared/errors"
	"internal-developer-platform/src/shared/kernel/capabilities/authz"
)

type contextKey string

const SubjectContextKey contextKey = "rbac_subject"

type RBACMiddleware struct {
	authorizer authz.Authorizer
}

func NewRBACMiddleware(authorizer authz.Authorizer) *RBACMiddleware {
	return &RBACMiddleware{authorizer: authorizer}
}

func (m *RBACMiddleware) RequirePermission(resource authz.Resource, action authz.Action, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m.authorizer == nil {
			http.Error(w, `{"error":"authorization capability unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		authHeader := r.Header.Get(constants.HeaderAuthorization)
		if authHeader == "" || !strings.HasPrefix(authHeader, constants.BearerPrefix) {
			http.Error(w, constants.ErrMissingAuthHeaderJSON, http.StatusUnauthorized)
			return
		}

		rawToken := strings.TrimPrefix(authHeader, constants.BearerPrefix)
		targetTenant := r.Header.Get(constants.HeaderXTenantID)
		evalReq := buildRouteCheckRequestFromHTTP(r, targetTenant, resource, action)

		subject, err := m.authorizer.AuthenticateAndAuthorizeHTTP(r.Context(), rawToken, evalReq)
		if err != nil {
			status := http.StatusForbidden
			if sharedErrors.IsUnauthorizedError(err) {
				status = http.StatusUnauthorized
			}
			http.Error(w, `{"error":"`+err.Error()+`"}`, status)
			return
		}

		if impersonateUser := r.Header.Get(constants.HeaderXImpersonateUser); impersonateUser != "" {
			callerMTLS := r.TLS != nil && len(r.TLS.PeerCertificates) > 0
			hasGrant := subjectHasImpersonateGrant(*subject)
			targetProtected := strings.EqualFold(impersonateUser, "PlatformAdmin") || strings.EqualFold(impersonateUser, "admin")
			if err := m.authorizer.VerifyImpersonation(r.Context(), callerMTLS, hasGrant, targetProtected); err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusForbidden)
				return
			}
		}

		if delegationTenant := r.Header.Get(constants.HeaderXDelegationTenant); delegationTenant != "" {
			if err := m.authorizer.VerifyDelegationScope(r.Context(), true, subject.TenantID, delegationTenant); err != nil {
				http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusForbidden)
				return
			}
		}

		ctx := context.WithValue(r.Context(), SubjectContextKey, subject)
		next(w, r.WithContext(ctx))
	}
}

func GetSubjectFromContext(ctx context.Context) *authz.SubjectContext {
	val := ctx.Value(SubjectContextKey)
	if val == nil {
		return nil
	}
	if sub, ok := val.(*authz.SubjectContext); ok {
		return sub
	}
	return nil
}

func buildRouteCheckRequestFromHTTP(r *http.Request, targetTenant string, resource authz.Resource, action authz.Action) authz.RouteCheckRequest {
	req := authz.RouteCheckRequest{
		TargetTenantID:         targetTenant,
		Resource:               resource,
		Action:                 action,
		TargetEnvironment:      r.Header.Get(constants.HeaderXEnvironment),
		ResourceClassification: r.Header.Get(constants.HeaderXResourceClass),
		ChangeFreezeActive:     strings.EqualFold(r.Header.Get(constants.HeaderXChangeFreeze), "true"),
		RequireStepUpMFA:       strings.EqualFold(r.Header.Get(constants.HeaderXRequireStepUpMFA), "true"),
		GlobalAuthorityDown:    strings.EqualFold(r.Header.Get(constants.HeaderXGlobalAuthorityDown), "true"),
	}
	if ageSec, err := strconv.ParseInt(r.Header.Get(constants.HeaderXPolicySnapshotAgeSec), 10, 64); err == nil && ageSec > 0 {
		req.PolicySnapshotAgeSec = ageSec
	}
	if ticketID := r.Header.Get(constants.HeaderXBreakGlassTicket); ticketID != "" {
		ttlSeconds, _ := strconv.Atoi(r.Header.Get(constants.HeaderXBreakGlassTTL))
		req.BreakGlassTicketID = ticketID
		req.BreakGlassApproved = strings.EqualFold(r.Header.Get(constants.HeaderXBreakGlassApproved), "true")
		req.BreakGlassTier0Prod = strings.EqualFold(r.Header.Get(constants.HeaderXBreakGlassTier0), "true")
		req.BreakGlassDualAuth = strings.EqualFold(r.Header.Get(constants.HeaderXBreakGlassDualAuth), "true")
		req.BreakGlassTTLSeconds = ttlSeconds
	}
	return req
}

func subjectHasImpersonateGrant(sub authz.SubjectContext) bool {
	for _, p := range sub.Permissions {
		if strings.EqualFold(string(p.Resource), "iam") && strings.EqualFold(string(p.Action), "impersonate") {
			return true
		}
		if p.Resource == authz.ResourceAll && p.Action == authz.ActionAll {
			return true
		}
	}
	return false
}
