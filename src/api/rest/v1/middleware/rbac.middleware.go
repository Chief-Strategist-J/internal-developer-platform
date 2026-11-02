package middleware

import (
	"context"
	"net/http"
	"strings"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
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
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error":"missing or invalid authorization header"}`, http.StatusUnauthorized)
			return
		}

		rawToken := strings.TrimPrefix(authHeader, "Bearer ")
		targetTenant := r.Header.Get("X-Tenant-ID")

		subject, err := m.rbacSvc.AuthenticateAndAuthorize(r.Context(), rawToken, targetTenant, resource, action)
		if err != nil {
			status := http.StatusForbidden
			if strings.Contains(strings.ToLower(err.Error()), "token") ||
				strings.Contains(strings.ToLower(err.Error()), "signature") ||
				strings.Contains(strings.ToLower(err.Error()), "revoked") ||
				strings.Contains(strings.ToLower(err.Error()), "unauthorized") {
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
