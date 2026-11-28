/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DEPLOYMENT STRATEGIES REST ROUTER (V.3 role 01)

1. Purpose:
   - Registers deployment strategies HTTP routes under ServeMux.
   - Enforces RBAC & Idempotency middleware integration.
2. Invariants:
   - Zero inline comments below header.
*/

package restv1

import (
	"net/http"

	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/kernel"
	"internal-developer-platform/src/shared/kernel/capabilities/authz"
)

type DeploymentStrategiesRouter struct {
	handler     *DeploymentStrategiesHandler
	rbacMiddle  *middleware.RBACMiddleware
	idempMiddle *middleware.IdempotencyMiddleware
}

func NewDeploymentStrategiesRouter(
	handler *DeploymentStrategiesHandler,
	rbacM *middleware.RBACMiddleware,
	idempM *middleware.IdempotencyMiddleware,
) *DeploymentStrategiesRouter {
	return &DeploymentStrategiesRouter{
		handler:     handler,
		rbacMiddle:  rbacM,
		idempMiddle: idempM,
	}
}

func (r *DeploymentStrategiesRouter) RegisterRoutes(mux *http.ServeMux) {
	plansHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.handler.HandleCreatePlan(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	rolloutsHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			r.handler.HandleStartRollout(w, req)
		case http.MethodGet:
			r.handler.HandleGetRollout(w, req)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	if r.idempMiddle != nil {
		plansHandler = r.idempMiddle.Wrap(plansHandler)
		rolloutsHandler = r.idempMiddle.Wrap(rolloutsHandler)
	}

	if r.rbacMiddle != nil {
		mux.HandleFunc("/api/v1/deployment-strategies/plans", r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			plansHandler,
		))
		mux.HandleFunc("/api/v1/deployment-strategies/rollouts", r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			rolloutsHandler,
		))
		mux.HandleFunc("/api/v1/deployment-strategies/rollouts/", r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionRead,
			rolloutsHandler,
		))
		return
	}

	mux.HandleFunc("/api/v1/deployment-strategies/plans", plansHandler)
	mux.HandleFunc("/api/v1/deployment-strategies/rollouts", rolloutsHandler)
	mux.HandleFunc("/api/v1/deployment-strategies/rollouts/", rolloutsHandler)
}

func (r *DeploymentStrategiesRouter) KernelRoutes() []kernel.Route {
	return []kernel.Route{
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/deployment-strategies/plans",
			Handler: http.HandlerFunc(r.handler.HandleCreatePlan),
		},
		{
			Method:  http.MethodPost,
			Pattern: "/api/v1/deployment-strategies/rollouts",
			Handler: http.HandlerFunc(r.handler.HandleStartRollout),
		},
		{
			Method:  http.MethodGet,
			Pattern: "/api/v1/deployment-strategies/rollouts/{id}",
			Handler: http.HandlerFunc(r.handler.HandleGetRollout),
		},
	}
}
