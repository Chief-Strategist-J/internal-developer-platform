/*
ALGORITHM & ARCHITECTURE BLUEPRINT: KUBERNETES WORKLOAD INGRESS ROUTER

1. Routing Flow Execution:
  - Registers declarative endpoints for Kubernetes Workload Provisioning API.
  - Binds root endpoint constants from shared system constants.
  - Attaches Role-Based Access Control (RBAC) permission evaluation when middleware is supplied.
  - Segregates collection endpoints (POST /api/v1/workloads) from entity lookups (GET /api/v1/workloads/{id}).

2. Invariants:
  - Zero inline comments inside function bodies.
  - All functions strictly maintain a length <= 20 lines.
  - All endpoint paths reference centralized package constants.
*/
package routers

import (
	"net/http"
	"strings"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/middleware"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/shared/constants"
)

type KubernetesWorkloadRouter struct {
	handler    *handlers.KubernetesWorkloadHandler
	rbacMiddle *middleware.RBACMiddleware
}

func NewKubernetesWorkloadRouter(h *handlers.KubernetesWorkloadHandler, m *middleware.RBACMiddleware) *KubernetesWorkloadRouter {
	return &KubernetesWorkloadRouter{
		handler:    h,
		rbacMiddle: m,
	}
}

func (r *KubernetesWorkloadRouter) RegisterRoutes(mux *http.ServeMux) {
	workloadHandler := r.buildWorkloadHandler()
	singleWorkloadHandler := r.buildSingleWorkloadHandler()
	timelineHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			r.handler.RecordTimelineEvent(w, req)
		case http.MethodGet:
			r.handler.ListTimelineEvents(w, req)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	if r.rbacMiddle != nil {
		r.registerSecuredRoutes(mux, workloadHandler, singleWorkloadHandler)
		mux.HandleFunc(constants.EndpointTimelinePrefix, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceInfrastructure,
			rbacContracts.ActionRead,
			timelineHandler,
		))
		return
	}
	r.registerUnsecuredRoutes(mux, workloadHandler, singleWorkloadHandler)
	mux.HandleFunc(constants.EndpointTimelinePrefix, timelineHandler)
}

func (r *KubernetesWorkloadRouter) buildWorkloadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.handler.ProvisionWorkload(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *KubernetesWorkloadRouter) buildSingleWorkloadHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, constants.EndpointWorkloadsPrefix) && req.Method == http.MethodGet {
			r.handler.GetWorkload(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *KubernetesWorkloadRouter) registerSecuredRoutes(mux *http.ServeMux, wh, swh http.HandlerFunc) {
	mux.HandleFunc(constants.EndpointWorkloads, r.rbacMiddle.RequirePermission(
		rbacContracts.ResourceServiceCatalog,
		rbacContracts.ActionWrite,
		wh,
	))
	mux.HandleFunc(constants.EndpointWorkloadsPrefix, r.rbacMiddle.RequirePermission(
		rbacContracts.ResourceServiceCatalog,
		rbacContracts.ActionRead,
		swh,
	))
}

func (r *KubernetesWorkloadRouter) registerUnsecuredRoutes(mux *http.ServeMux, wh, swh http.HandlerFunc) {
	mux.HandleFunc(constants.EndpointWorkloads, wh)
	mux.HandleFunc(constants.EndpointWorkloadsPrefix, swh)
}
