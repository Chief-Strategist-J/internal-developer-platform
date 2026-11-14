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
	workloadHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			r.handler.ProvisionWorkload(w, req)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	singleWorkloadHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, constants.EndpointWorkloadsPrefix) && req.Method == http.MethodGet {
			r.handler.GetWorkload(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	if r.rbacMiddle != nil {
		mux.HandleFunc(constants.EndpointWorkloads, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceServiceCatalog,
			rbacContracts.ActionWrite,
			workloadHandler,
		))
		mux.HandleFunc(constants.EndpointWorkloadsPrefix, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceServiceCatalog,
			rbacContracts.ActionRead,
			singleWorkloadHandler,
		))
	} else {
		mux.HandleFunc(constants.EndpointWorkloads, workloadHandler)
		mux.HandleFunc(constants.EndpointWorkloadsPrefix, singleWorkloadHandler)
	}
}

