package routers

import (
	"net/http"

	"internal-developer-platform/src/api/rest/v1/handlers"
)

type HealthCheckRouter struct {
	handler *handlers.HealthCheckHandler
}

func NewHealthCheckRouter(h *handlers.HealthCheckHandler) *HealthCheckRouter {
	return &HealthCheckRouter{handler: h}
}

func (r *HealthCheckRouter) RegisterAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			r.handler.HandleLiveness(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/livez", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			r.handler.HandleLiveness(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			r.handler.HandleReadiness(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			r.handler.HandleLiveness(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/health/ready", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			r.handler.HandleReadiness(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
}

func (r *HealthCheckRouter) RegisterPublicRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			r.handler.HandleLiveness(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			r.handler.HandleReadiness(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.HandleFunc("/api/v1/health", func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			r.handler.HandleSystemHealth(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
}
