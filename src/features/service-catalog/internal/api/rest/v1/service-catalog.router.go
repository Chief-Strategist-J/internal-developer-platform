package restv1

import (
	"net/http"

	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/kernel/capabilities/authz"
)

type ServiceCatalogRouter struct {
	handler    *ServiceCatalogHandler
	rbacMiddle *middleware.RBACMiddleware
}

func NewServiceCatalogRouter(h *ServiceCatalogHandler, m *middleware.RBACMiddleware) *ServiceCatalogRouter {
	return &ServiceCatalogRouter{
		handler:    h,
		rbacMiddle: m,
	}
}

func (r *ServiceCatalogRouter) RegisterRoutes(mux *http.ServeMux) {
	servicesHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			r.handler.RegisterService(w, req)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	if r.rbacMiddle != nil {
		mux.HandleFunc(constants.EndpointServices, r.rbacMiddle.RequirePermission(
			authz.ResourceServiceCatalog,
			authz.ActionWrite,
			servicesHandler,
		))
	} else {
		mux.HandleFunc(constants.EndpointServices, servicesHandler)
	}
}
