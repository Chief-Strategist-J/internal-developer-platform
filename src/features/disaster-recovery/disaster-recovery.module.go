/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DISASTER RECOVERY MODULE (V.3 role 15, X-05)

 1. Role:
    - Assembles Postgres store, cloud DR adapters, REST handler/router, and ReadinessProberLifecycle.
 2. Invariants:
    - Zero goroutines or tickers created during BuildModule (G-11).
    - Zero comments below this header.
*/
package disasterrecovery

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	cliv1 "internal-developer-platform/src/features/disaster-recovery/internal/api/cli/v1"
	restv1 "internal-developer-platform/src/features/disaster-recovery/internal/api/rest/v1"
	"internal-developer-platform/src/features/disaster-recovery/internal/infrastructure/adapters"
	drLifecycle "internal-developer-platform/src/features/disaster-recovery/internal/infrastructure/lifecycle"
	"internal-developer-platform/src/features/disaster-recovery/internal/service"
	"internal-developer-platform/src/infra/config"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/kernel"
)

type ModuleDeps struct {
	DB             *sql.DB
	Config         *config.AppConfig
	RBACMiddleware *middleware.RBACMiddleware
}

type DisasterRecoveryModule struct {
	svc        *service.DisasterRecoveryService
	bridge     DRBridgeProvider
	handler    *restv1.DisasterRecoveryHandler
	router     *restv1.DisasterRecoveryRouter
	cliHandler *cliv1.DisasterRecoveryCLIHandler
	lifecycles []kernel.Lifecycle
}

type drBridgeProviderImpl struct {
	svc *service.DisasterRecoveryService
}

func (b *drBridgeProviderImpl) IsWorkloadFenced(ctx context.Context, tenantID, workloadID string) (bool, error) {
	if b.svc == nil {
		return false, nil
	}
	epoch, err := b.svc.GetFencingEpoch(ctx, workloadID)
	if err != nil {
		return false, nil
	}
	if epoch != nil && epoch.Epoch > 0 {
		return true, nil
	}
	return false, nil
}

func (b *drBridgeProviderImpl) GetFencingEpoch(ctx context.Context, datasetID string) (int64, error) {
	if b.svc == nil {
		return 0, nil
	}
	rec, err := b.svc.GetFencingEpoch(ctx, datasetID)
	if err != nil || rec == nil {
		return 0, err
	}
	return rec.Epoch, nil
}

func BuildModule(deps ModuleDeps) *DisasterRecoveryModule {
	store := adapters.NewPostgresDRRepositoryAdapter(deps.DB)
	cloud := adapters.NewCloudProviderDRAdapter()
	svc := service.NewDisasterRecoveryService(store, cloud, cloud, cloud, cloud, cloud)
	handler := restv1.NewDisasterRecoveryHandler(svc)
	router := restv1.NewDisasterRecoveryRouter(handler, deps.RBACMiddleware)
	bridge := &drBridgeProviderImpl{svc: svc}
	var cliBaseURL, cliTenantID, cliToken string
	if deps.Config != nil {
		cliBaseURL = deps.Config.DRCLI.APIBaseURL
		cliTenantID = deps.Config.DRCLI.TenantID
		cliToken = deps.Config.DRCLI.AuthToken
	}
	cliHandler := cliv1.NewDisasterRecoveryCLIHandlerWithConfig(cliBaseURL, cliTenantID, cliToken)
	lifecycles := []kernel.Lifecycle{
		drLifecycle.NewReadinessProberLifecycle(svc, 30*time.Second),
	}
	return &DisasterRecoveryModule{
		svc:        svc,
		bridge:     bridge,
		handler:    handler,
		router:     router,
		cliHandler: cliHandler,
		lifecycles: lifecycles,
	}
}

func (m *DisasterRecoveryModule) Name() string                         { return "disaster-recovery" }
func (m *DisasterRecoveryModule) Routes() []kernel.Route               { return nil }
func (m *DisasterRecoveryModule) EventHandlers() []kernel.EventHandler { return nil }
func (m *DisasterRecoveryModule) Lifecycles() []kernel.Lifecycle       { return m.lifecycles }
func (m *DisasterRecoveryModule) Provides() []string                   { return nil }
func (m *DisasterRecoveryModule) BridgeProvider() DRBridgeProvider     { return m.bridge }

func (m *DisasterRecoveryModule) ExecuteCLI(ctx context.Context, args []string) int {
	if m.cliHandler == nil {
		return 1
	}
	return m.cliHandler.Execute(ctx, args)
}

func (m *DisasterRecoveryModule) RegisterRoutes(mux *http.ServeMux) {
	if m.router != nil {
		m.router.RegisterRoutes(mux)
	}
}
