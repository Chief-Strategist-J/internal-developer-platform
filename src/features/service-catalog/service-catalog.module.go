/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SERVICE CATALOG MODULE (V.3 role 15, X-05)

 1. Role:
    - Assembles Postgres catalog adapter, ServiceCatalogService, REST handler, and router.
 2. Invariants:
    - Zero goroutines or tickers in constructors (G-11).
    - Zero comments below this header.
*/
package servicecatalog

import (
	"database/sql"
	"net/http"

	restv1 "internal-developer-platform/src/features/service-catalog/internal/api/rest/v1"
	"internal-developer-platform/src/features/service-catalog/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/service-catalog/internal/service"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/kernel"
)

type ModuleDeps struct {
	DB             *sql.DB
	RBACMiddleware *middleware.RBACMiddleware
}

type ServiceCatalogModule struct {
	svc     *service.ServiceCatalogService
	handler *restv1.ServiceCatalogHandler
	router  *restv1.ServiceCatalogRouter
}

func BuildModule(deps ModuleDeps) *ServiceCatalogModule {
	repo := adapters.NewPostgresServiceCatalogAdapter(deps.DB)
	svc := service.NewServiceCatalogService(repo)
	handler := restv1.NewServiceCatalogHandler(svc)
	router := restv1.NewServiceCatalogRouter(handler, deps.RBACMiddleware)
	return &ServiceCatalogModule{
		svc:     svc,
		handler: handler,
		router:  router,
	}
}

func (m *ServiceCatalogModule) Name() string                         { return "service-catalog" }
func (m *ServiceCatalogModule) Routes() []kernel.Route               { return nil }
func (m *ServiceCatalogModule) EventHandlers() []kernel.EventHandler { return nil }
func (m *ServiceCatalogModule) Lifecycles() []kernel.Lifecycle       { return nil }
func (m *ServiceCatalogModule) Provides() []string                   { return nil }

func (m *ServiceCatalogModule) RegisterRoutes(mux *http.ServeMux) {
	if m.router != nil {
		m.router.RegisterRoutes(mux)
	}
}
