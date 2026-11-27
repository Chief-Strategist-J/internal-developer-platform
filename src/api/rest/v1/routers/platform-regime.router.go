/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PLATFORM REGIME REST ROUTER

1. Routing Flow Execution:
  - Registers declarative endpoints for Platform Regime API.
  - Binds /api/v1/regimes (POST) and /api/v1/regimes/active (GET).
  - Enforces RBAC permissions via RequirePermission: ResourceInfrastructure ActionWrite for register, ActionRead for active.
  - Zero inline comments inside function bodies.
*/
package routers

import (
	"net/http"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/kernel/capabilities/authz"
)

type PlatformRegimeRouter struct {
	handler    *handlers.PlatformRegimeHandler
	rbacMiddle *middleware.RBACMiddleware
}

func NewPlatformRegimeRouter(h *handlers.PlatformRegimeHandler, m *middleware.RBACMiddleware) *PlatformRegimeRouter {
	return &PlatformRegimeRouter{
		handler:    h,
		rbacMiddle: m,
	}
}

func (r *PlatformRegimeRouter) RegisterRoutes(mux *http.ServeMux) {
	regimeHandler := r.buildRegimeHandler()
	activeHandler := r.buildActiveRegimeHandler()

	if r.rbacMiddle != nil {
		mux.HandleFunc(constants.EndpointRegimes, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			regimeHandler,
		))
		mux.HandleFunc(constants.EndpointRegimesActive, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionRead,
			activeHandler,
		))
		return
	}

	mux.HandleFunc(constants.EndpointRegimes, regimeHandler)
	mux.HandleFunc(constants.EndpointRegimesActive, activeHandler)
}

func (r *PlatformRegimeRouter) buildRegimeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.handler.RegisterRegimePolicy(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *PlatformRegimeRouter) buildActiveRegimeHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			r.handler.GetActiveRegimePolicy(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
