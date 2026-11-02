package routers

import (
	"net/http"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/middleware"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
)

type ServiceCatalogRouter struct {
	handler    *handlers.ServiceCatalogHandler
	rbacMiddle *middleware.RBACMiddleware
}

func NewServiceCatalogRouter(h *handlers.ServiceCatalogHandler, m *middleware.RBACMiddleware) *ServiceCatalogRouter {
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
		mux.HandleFunc("/api/v1/services", r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceServiceCatalog,
			rbacContracts.ActionWrite,
			servicesHandler,
		))
	} else {
		mux.HandleFunc("/api/v1/services", servicesHandler)
	}
}
