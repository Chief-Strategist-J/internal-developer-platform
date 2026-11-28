/*
ALGORITHM & ARCHITECTURE BLUEPRINT: AUTOSCALING REST ROUTER (V.3 role 01, API Instruction 3)

1. Purpose:
   - Mounts the evaluate endpoint (path from shared constants, mirrored in config/endpoints.yaml) behind
     RBAC (infrastructure:scale) and idempotency middleware when supplied.
2. Invariants:
   - Zero business logic; route paths stay in step with contracts/openapi/v1.yaml.
   - Zero inline comments below this header.
*/

package restv1

import (
	"net/http"

	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/kernel"
	"internal-developer-platform/src/shared/kernel/capabilities/authz"
)

type AutoscalingRouter struct {
	handler     *AutoscalingHandler
	rbacMiddle  *middleware.RBACMiddleware
	idempMiddle *middleware.IdempotencyMiddleware
}

func NewAutoscalingRouter(
	handler *AutoscalingHandler,
	rbacM *middleware.RBACMiddleware,
	idempM *middleware.IdempotencyMiddleware,
) *AutoscalingRouter {
	return &AutoscalingRouter{handler: handler, rbacMiddle: rbacM, idempMiddle: idempM}
}

func (r *AutoscalingRouter) RegisterRoutes(mux *http.ServeMux) {
	evaluate := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		r.handler.HandleEvaluatePlan(w, req)
	})
	if r.idempMiddle != nil {
		evaluate = r.idempMiddle.Wrap(evaluate)
	}
	if r.rbacMiddle != nil {
		evaluate = r.rbacMiddle.RequirePermission(authz.ResourceInfrastructure, authz.ActionScale, evaluate)
	}
	mux.HandleFunc(constants.EndpointAutoscalingEvaluatePlan, evaluate)
}

func (r *AutoscalingRouter) KernelRoutes() []kernel.Route {
	return []kernel.Route{
		{
			Method:  http.MethodPost,
			Pattern: constants.EndpointAutoscalingEvaluatePlan,
			Handler: http.HandlerFunc(r.handler.HandleEvaluatePlan),
		},
	}
}
