/*
ALGORITHM & ARCHITECTURE BLUEPRINT: HEALTH CHECKS MODULE (V.3 role 15, X-05)

 1. Role:
    - Assembles HealthCheckService, REST handler, and public + admin routers.
 2. Invariants:
    - Zero goroutines or tickers in constructors (G-11).
    - Zero comments below this header.
*/
package healthchecks

import (
	"net/http"

	restv1 "internal-developer-platform/src/features/health-checks/internal/api/rest/v1"
	"internal-developer-platform/src/features/health-checks/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/health-checks/internal/service"
	"internal-developer-platform/src/shared/kernel"
)

type HealthChecksModule struct {
	svc     *service.HealthCheckService
	handler *restv1.HealthCheckHandler
	router  *restv1.HealthCheckRouter
}

func BuildModule() *HealthChecksModule {
	repo := adapters.NewMemoryHealthCheckerAdapter()
	svc := service.NewHealthCheckService(repo)
	handler := restv1.NewHealthCheckHandler(svc)
	router := restv1.NewHealthCheckRouter(handler)
	return &HealthChecksModule{
		svc:     svc,
		handler: handler,
		router:  router,
	}
}

func (m *HealthChecksModule) Name() string                         { return "health-checks" }
func (m *HealthChecksModule) Routes() []kernel.Route               { return nil }
func (m *HealthChecksModule) EventHandlers() []kernel.EventHandler { return nil }
func (m *HealthChecksModule) Lifecycles() []kernel.Lifecycle       { return nil }
func (m *HealthChecksModule) Provides() []string                   { return nil }

func (m *HealthChecksModule) RegisterPublicRoutes(mux *http.ServeMux) {
	if m.router != nil {
		m.router.RegisterPublicRoutes(mux)
	}
}

func (m *HealthChecksModule) RegisterAdminRoutes(mux *http.ServeMux) {
	if m.router != nil {
		m.router.RegisterAdminRoutes(mux)
	}
}
