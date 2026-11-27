/*
ALGORITHM & ARCHITECTURE BLUEPRINT: POLICY AS CODE MODULE (V.3 role 15, X-05)

 1. Role:
    - Assembles Postgres policy repository, PolicyAsCodeService, bridge adapter, REST handler, and router.
 2. Invariants:
    - Zero goroutines or tickers in constructors (G-11); never exports internal Service (X-05).
    - Zero comments below this header.
*/
package policyascode

import (
	"context"
	"database/sql"
	"net/http"

	restv1 "internal-developer-platform/src/features/policy-as-code/internal/api/rest/v1"
	"internal-developer-platform/src/features/policy-as-code/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/policy-as-code/internal/service"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/kernel"
)

type ModuleDeps struct {
	DB             *sql.DB
	RBACMiddleware *middleware.RBACMiddleware
}

type policyBridgeAdapter struct {
	svc *service.PolicyAsCodeService
}

func (a *policyBridgeAdapter) GetActiveExceptionForWorkload(ctx context.Context, tenantID string, namespace string, workloadName string) (*ActiveWorkloadException, error) {
	exc, err := a.svc.GetActiveExceptionForWorkload(ctx, tenantID, namespace, workloadName)
	if err != nil || exc == nil {
		return nil, err
	}
	return &ActiveWorkloadException{
		PublicID:   exc.PublicID,
		ApproverID: exc.ApproverID,
	}, nil
}

type PolicyAsCodeModule struct {
	svc     *service.PolicyAsCodeService
	bridge  PolicyBridgeProvider
	handler *restv1.PolicyAsCodeHandler
	router  *restv1.PolicyAsCodeRouter
}

func BuildModule(deps ModuleDeps) *PolicyAsCodeModule {
	repo := adapters.NewPostgresPolicyRepositoryAdapter(deps.DB)
	svc := service.NewPolicyAsCodeService(repo)
	handler := restv1.NewPolicyAsCodeHandler(svc)
	router := restv1.NewPolicyAsCodeRouter(handler, deps.RBACMiddleware)
	return &PolicyAsCodeModule{
		svc:     svc,
		bridge:  &policyBridgeAdapter{svc: svc},
		handler: handler,
		router:  router,
	}
}

func (m *PolicyAsCodeModule) Name() string                         { return "policy-as-code" }
func (m *PolicyAsCodeModule) Routes() []kernel.Route               { return nil }
func (m *PolicyAsCodeModule) EventHandlers() []kernel.EventHandler { return nil }
func (m *PolicyAsCodeModule) Lifecycles() []kernel.Lifecycle       { return nil }
func (m *PolicyAsCodeModule) Provides() []string                   { return nil }
func (m *PolicyAsCodeModule) BridgeProvider() PolicyBridgeProvider { return m.bridge }

func (m *PolicyAsCodeModule) RegisterRoutes(mux *http.ServeMux) {
	if m.router != nil {
		m.router.RegisterRoutes(mux)
	}
}
