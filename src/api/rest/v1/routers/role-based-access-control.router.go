package routers

import (
	"net/http"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/middleware"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/shared/constants"
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
	mux.HandleFunc(constants.EndpointAuthToken, func(w http.ResponseWriter, req *http.Request) {
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
	mux.HandleFunc(constants.EndpointAuthMe, r.rbacMiddle.RequirePermission(
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
	mux.HandleFunc(constants.EndpointAuthRevoke, r.rbacMiddle.RequirePermission(
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
	mux.HandleFunc(constants.EndpointAuthEvaluate, r.rbacMiddle.RequirePermission(
		rbacContracts.ResourceAuth,
		rbacContracts.ActionRead,
		evalHandler,
	))

	jitHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			r.handler.RequestJITAccess(w, req)
		case http.MethodGet:
			r.handler.ListActiveGrants(w, req)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(constants.EndpointAuthJITGrants, r.rbacMiddle.RequirePermission(
		rbacContracts.ResourceAuth,
		rbacContracts.ActionRead,
		jitHandler,
	))

	revokeJitHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.handler.RevokeJITAccess(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.HandleFunc(constants.EndpointAuthJITGrantsPrefix, r.rbacMiddle.RequirePermission(
		rbacContracts.ResourceAll,
		rbacContracts.ActionAll,
		revokeJitHandler,
	))

	bindingHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			r.handler.BindUserRole(w, req)
		case http.MethodDelete:
			r.handler.UnbindUserRole(w, req)
		case http.MethodGet:
			r.handler.GetUserRoles(w, req)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc(constants.EndpointAuthBindings, r.rbacMiddle.RequirePermission(
		rbacContracts.ResourceAuth,
		rbacContracts.ActionWrite,
		bindingHandler,
	))

	bindingSubjectHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			r.handler.GetUserRoles(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.HandleFunc(constants.EndpointAuthBindingsPrefix, r.rbacMiddle.RequirePermission(
		rbacContracts.ResourceAuth,
		rbacContracts.ActionRead,
		bindingSubjectHandler,
	))
}
