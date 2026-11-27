// Package gitopsreconciliation: gitops-reconciliation.module assembles the GitOps module (V.3 role 15, X-05).
//
// Blueprint:
//  1. Constructs Postgres store, ArgoCD client, secret adapter, outbox port, handler, router, and outbox-relay lifecycle.
//  2. Consumes RBAC's task-lease capability indirectly via SetTaskLeasePort (X-03).
//
// Invariants:
//   - No raw goroutines or tickers created during BuildModule (G-11).
package gitopsreconciliation

import (
	"database/sql"
	"net/http"
	"time"

	restv1 "internal-developer-platform/src/features/gitops-reconciliation/api/rest/v1"
	"internal-developer-platform/src/features/gitops-reconciliation/infrastructure/adapters"
	gitopsLifecycle "internal-developer-platform/src/features/gitops-reconciliation/infrastructure/lifecycle"
	"internal-developer-platform/src/features/gitops-reconciliation/infrastructure/ports"
	"internal-developer-platform/src/features/gitops-reconciliation/service"
	argocdClientPkg "internal-developer-platform/src/infra/clients/argocd"
	"internal-developer-platform/src/infra/clients/resilience"
	"internal-developer-platform/src/infra/config"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/kernel"
)

type ModuleDeps struct {
	DB             *sql.DB
	Config         *config.AppConfig
	ResilienceCfg  resilience.ResilienceConfig
	RBACMiddleware *middleware.RBACMiddleware
	TaskLeasePort  ports.TaskLeaseGuardPort
}

type GitOpsModule struct {
	svc        *service.GitOpsReconciliationService
	handler    *restv1.GitOpsReconciliationHandler
	router     *restv1.GitOpsReconciliationRouter
	lifecycles []kernel.Lifecycle
}

func BuildModule(deps ModuleDeps) *GitOpsModule {
	store := adapters.NewPostgresGitOpsRepositoryAdapter(deps.DB)
	var argoClient ports.ArgoCDClientPort
	if deps.Config.ArgoCD.Endpoint != "" && deps.Config.ArgoCD.Token != "" {
		liveArgo := argocdClientPkg.NewArgoCDClient(argocdClientPkg.ArgoCDClientConfig{
			BaseURL:   deps.Config.ArgoCD.Endpoint,
			AuthToken: deps.Config.ArgoCD.Token,
			Timeout:   15 * time.Second,
		}, deps.ResilienceCfg)
		argoClient = adapters.NewLiveArgoCDClientAdapter(liveArgo)
	} else {
		argoClient = adapters.NewArgoCDClientAdapter(deps.Config.ArgoCD.Endpoint, deps.Config.ArgoCD.Token)
	}
	svc := service.NewGitOpsReconciliationService(store, argoClient)
	svc.SetSecretAdapter(adapters.NewConfigurableSecretAdapterWithDB(deps.DB))
	svc.SetOutboxPort(adapters.NewPostgresOutboxAdapter(deps.DB))
	if deps.TaskLeasePort != nil {
		svc.SetTaskLeasePort(deps.TaskLeasePort)
	}
	handler := restv1.NewGitOpsReconciliationHandler(svc)
	router := restv1.NewGitOpsReconciliationRouter(handler, deps.RBACMiddleware)
	var lcs []kernel.Lifecycle
	if deps.DB != nil {
		lcs = append(lcs, gitopsLifecycle.NewGitOpsOutboxRelayLifecycle(svc, 5*time.Second))
	}
	return &GitOpsModule{
		svc:        svc,
		handler:    handler,
		router:     router,
		lifecycles: lcs,
	}
}

func (m *GitOpsModule) Name() string                                  { return "gitops-reconciliation" }
func (m *GitOpsModule) Routes() []kernel.Route                        { return nil }
func (m *GitOpsModule) EventHandlers() []kernel.EventHandler          { return nil }
func (m *GitOpsModule) Lifecycles() []kernel.Lifecycle                { return m.lifecycles }
func (m *GitOpsModule) Provides() []string                            { return nil }
func (m *GitOpsModule) Service() *service.GitOpsReconciliationService { return m.svc }

func (m *GitOpsModule) RegisterRoutes(mux *http.ServeMux) {
	if m.router != nil {
		m.router.RegisterRoutes(mux)
	}
}
