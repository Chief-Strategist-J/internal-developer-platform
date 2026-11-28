/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DEPLOYMENT STRATEGIES MODULE (V.3 role 15, X-05)

1. Purpose:
   - Module composition root assembling adapters, domain service, REST handler, and router.
   - Conforms to shared kernel.Module interface.
2. Invariants:
   - Zero goroutines, tickers, or connections started in BuildModule (G-11).
   - Zero internal services or ports exported outside the feature boundary (X-05).
   - Zero inline comments below header.
*/

package deploymentstrategies

import (
	"database/sql"
	"net/http"

	restv1 "internal-developer-platform/src/features/deployment-strategies/internal/api/rest/v1"
	"internal-developer-platform/src/features/deployment-strategies/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/deployment-strategies/internal/infrastructure/ports"
	"internal-developer-platform/src/features/deployment-strategies/internal/service"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/kernel"
)

type ModuleDeps struct {
	DB                    *sql.DB
	RBACMiddleware        *middleware.RBACMiddleware
	IdempotencyMiddleware *middleware.IdempotencyMiddleware
	DRGuard               DRGuardBridge
}

type DeploymentStrategiesModule struct {
	router *restv1.DeploymentStrategiesRouter
	routes []kernel.Route
}

func BuildModule(deps ModuleDeps) *DeploymentStrategiesModule {
	var repo ports.RepositoryPort
	if deps.DB != nil {
		repo = adapters.NewPostgresRepositoryAdapter(deps.DB)
	} else {
		repo = adapters.NewMemoryRepositoryAdapter()
	}

	traffic := adapters.NewMeshTrafficAdapter()
	orchestrator := adapters.NewArgoRolloutsAdapter()
	analysis := adapters.NewPrometheusAnalysisAdapter()

	svc := service.NewDeploymentStrategiesService(repo, traffic, orchestrator, analysis, deps.DRGuard)
	handler := restv1.NewDeploymentStrategiesHandler(svc)
	router := restv1.NewDeploymentStrategiesRouter(handler, deps.RBACMiddleware, deps.IdempotencyMiddleware)

	return &DeploymentStrategiesModule{
		router: router,
		routes: router.KernelRoutes(),
	}
}

func (m *DeploymentStrategiesModule) Name() string {
	return "deployment-strategies"
}

func (m *DeploymentStrategiesModule) Routes() []kernel.Route {
	return m.routes
}

func (m *DeploymentStrategiesModule) EventHandlers() []kernel.EventHandler {
	return nil
}

func (m *DeploymentStrategiesModule) Lifecycles() []kernel.Lifecycle {
	return nil
}

func (m *DeploymentStrategiesModule) Provides() []string {
	return []string{"progressive-delivery", "canary-rollout", "mesh-traffic"}
}

func (m *DeploymentStrategiesModule) RegisterRoutes(mux *http.ServeMux) {
	if m.router != nil {
		m.router.RegisterRoutes(mux)
	}
}
