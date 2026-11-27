// Package gitopsreconciliation: gitops-reconciliation.module assembles the GitOps module (V.3 role 15, X-05).
//
// Blueprint:
//  1. Constructs Postgres store, ArgoCD client, secret adapter, outbox port, handler, router, and outbox-relay lifecycle.
//  2. Consumes RBAC's task-lease capability via TaskLeaseBridge interface without exposing internal ports (X-03, GO-INT-001).
//
// Invariants:
//   - No raw goroutines or tickers created during BuildModule (G-11).
//   - Never exports internal Service (X-05).
package gitopsreconciliation

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	restv1 "internal-developer-platform/src/features/gitops-reconciliation/internal/api/rest/v1"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/infrastructure/adapters"
	gitopsLifecycle "internal-developer-platform/src/features/gitops-reconciliation/internal/infrastructure/lifecycle"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/infrastructure/ports"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/service"
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
	TaskLeasePort  TaskLeaseBridge
}

type taskLeasePortAdapter struct {
	bridge TaskLeaseBridge
}

func (a *taskLeasePortAdapter) EvaluateTaskLease(ctx context.Context, req ports.TaskLeaseRequest) (ports.TaskLeaseResult, error) {
	res, err := a.bridge.EvaluateTaskLease(ctx, TaskLeaseRequest{
		SubjectID:    req.SubjectID,
		TenantID:     req.TenantID,
		TaskType:     req.TaskType,
		LeaseToken:   req.LeaseToken,
		TokenExpired: req.TokenExpired,
		LeaseExpired: req.LeaseExpired,
	})
	return ports.TaskLeaseResult{
		Granted:   res.Granted,
		Reason:    res.Reason,
		Directive: res.Directive,
	}, err
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
		svc.SetTaskLeasePort(&taskLeasePortAdapter{bridge: deps.TaskLeasePort})
	}
	handler := restv1.NewGitOpsReconciliationHandlerWithSecret(svc, deps.Config.ArgoCD.WebhookSecret)
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

func (m *GitOpsModule) Name() string                         { return "gitops-reconciliation" }
func (m *GitOpsModule) Routes() []kernel.Route               { return nil }
func (m *GitOpsModule) EventHandlers() []kernel.EventHandler { return nil }
func (m *GitOpsModule) Lifecycles() []kernel.Lifecycle       { return m.lifecycles }
func (m *GitOpsModule) Provides() []string                   { return nil }

func (m *GitOpsModule) RegisterRoutes(mux *http.ServeMux) {
	if m.router != nil {
		m.router.RegisterRoutes(mux)
	}
}
