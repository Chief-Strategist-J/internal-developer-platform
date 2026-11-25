/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC INGRESS MIDDLEWARE

1. Ingress Execution:
  - Extracts Bearer JWT from Authorization header and X-Tenant-ID header.
  - Invokes RBACService.AuthenticateAndAuthorize under active request context.
  - Maps typed ApplicationError (401 Unauthorized vs 403 Forbidden) without string matching.
  - Zero mid-function inline comments.
*/
package middleware

import (
	"context"
	"net/http"
	"strings"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
	"internal-developer-platform/src/shared/constants"
	sharedErrors "internal-developer-platform/src/shared/errors"
)

type contextKey string

const SubjectContextKey contextKey = "rbac_subject"

type RBACMiddleware struct {
	rbacSvc *rbacService.RBACService
}

func NewRBACMiddleware(rbacSvc *rbacService.RBACService) *RBACMiddleware {
	return &RBACMiddleware{rbacSvc: rbacSvc}
}

func (m *RBACMiddleware) RequirePermission(resource rbacContracts.Resource, action rbacContracts.Action, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get(constants.HeaderAuthorization)
		if authHeader == "" || !strings.HasPrefix(authHeader, constants.BearerPrefix) {
			http.Error(w, constants.ErrMissingAuthHeaderJSON, http.StatusUnauthorized)
			return
		}

		rawToken := strings.TrimPrefix(authHeader, constants.BearerPrefix)
		targetTenant := r.Header.Get(constants.HeaderXTenantID)

		subject, err := m.rbacSvc.AuthenticateAndAuthorize(r.Context(), rawToken, targetTenant, resource, action)
		if err != nil {
			status := http.StatusForbidden
			if sharedErrors.IsUnauthorizedError(err) {
				status = http.StatusUnauthorized
			}
			http.Error(w, `{"error":"`+err.Error()+`"}`, status)
			return
		}

		ctx := context.WithValue(r.Context(), SubjectContextKey, subject)
		next(w, r.WithContext(ctx))
	}
}

func GetSubjectFromContext(ctx context.Context) *rbacContracts.Subject {
	val := ctx.Value(SubjectContextKey)
	if val == nil {
		return nil
	}
	if sub, ok := val.(*rbacContracts.Subject); ok {
		return sub
	}
	return nil
}
