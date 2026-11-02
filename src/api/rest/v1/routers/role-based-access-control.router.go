package routers

import (
	"net/http"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/middleware"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
)

type RBACRouter struct {
	handler    *handlers.RBACHandler
	rbacMiddle *middleware.RBACMiddleware
}

func NewRBACRouter(h *handlers.RBACHandler, m *middleware.RBACMiddleware) *RBACRouter {
	return &RBACRouter{
		handler:    h,
		rbacMiddle: m,
	}
}

func (r *RBACRouter) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/auth/token", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.handler.IssueToken(w, req)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	meHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			r.handler.GetCallerIdentity(w, req)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/v1/auth/me", r.rbacMiddle.RequirePermission(
		rbacContracts.ResourceAuth,
		rbacContracts.ActionRead,
		meHandler,
	))

	revokeHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.handler.RevokeSession(w, req)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/v1/auth/revoke", r.rbacMiddle.RequirePermission(
		rbacContracts.ResourceAll,
		rbacContracts.ActionAll,
		revokeHandler,
	))

	evalHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.handler.EvaluatePolicy(w, req)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/v1/auth/evaluate", r.rbacMiddle.RequirePermission(
		rbacContracts.ResourceAuth,
		rbacContracts.ActionRead,
		evalHandler,
	))
}
