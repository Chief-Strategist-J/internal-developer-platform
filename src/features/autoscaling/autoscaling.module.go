/*
ALGORITHM & ARCHITECTURE BLUEPRINT: AUTOSCALING MODULE (V.3 role 15, X-05)

1. Purpose:
   - Assembles the autoscaling feature (service, handler, router) and conforms to kernel.Module.
2. Invariants:
   - Pure construction: zero goroutines, tickers or connections (G-11).
   - No events or lifecycles are declared; the guardrails are an evaluation endpoint.
   - Zero inline comments below this header.
*/

package autoscaling

import (
	"net/http"

	restv1 "internal-developer-platform/src/features/autoscaling/internal/api/rest/v1"
	"internal-developer-platform/src/features/autoscaling/internal/service"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/kernel"
)

type ModuleDeps struct {
	RBACMiddleware        *middleware.RBACMiddleware
	IdempotencyMiddleware *middleware.IdempotencyMiddleware
}

type AutoscalingModule struct {
	router *restv1.AutoscalingRouter
	routes []kernel.Route
}

func BuildModule(deps ModuleDeps) *AutoscalingModule {
	svc := service.NewAutoscalingService()
	handler := restv1.NewAutoscalingHandler(svc)
	router := restv1.NewAutoscalingRouter(handler, deps.RBACMiddleware, deps.IdempotencyMiddleware)
	return &AutoscalingModule{router: router, routes: router.KernelRoutes()}
}

func (m *AutoscalingModule) Name() string {
	return "autoscaling"
}

func (m *AutoscalingModule) Routes() []kernel.Route {
	return m.routes
}

func (m *AutoscalingModule) RegisterRoutes(mux *http.ServeMux) {
	if m.router != nil {
		m.router.RegisterRoutes(mux)
	}
}

func (m *AutoscalingModule) EventHandlers() []kernel.EventHandler {
	return nil
}

func (m *AutoscalingModule) Lifecycles() []kernel.Lifecycle {
	return nil
}

func (m *AutoscalingModule) Provides() []string {
	return nil
}
