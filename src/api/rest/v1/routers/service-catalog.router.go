package routers

import (
	"net/http"

	"internal-developer-platform/src/api/rest/v1/handlers"
)

type ServiceCatalogRouter struct {
	handler *handlers.ServiceCatalogHandler
}

func NewServiceCatalogRouter(h *handlers.ServiceCatalogHandler) *ServiceCatalogRouter {
	return &ServiceCatalogRouter{handler: h}
}

func (r *ServiceCatalogRouter) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/services", func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			r.handler.RegisterService(w, req)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}
