/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PLATFORM REGIME MODULE (V.3 role 15, X-05)

 1. Role:
    - Assembles Postgres repository, baseline regime policy, service, bridge adapter, REST handler, and router.
 2. Invariants:
    - Zero goroutines or tickers in constructors (G-11); never exports internal Service (X-05).
    - Zero comments below this header.
*/
package platformregime

import (
	"context"
	"database/sql"
	"net/http"

	restv1 "internal-developer-platform/src/features/platform-regime/internal/api/rest/v1"
	"internal-developer-platform/src/features/platform-regime/internal/contracts"
	"internal-developer-platform/src/features/platform-regime/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/platform-regime/internal/service"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/kernel"
)

type ModuleDeps struct {
	DB             *sql.DB
	Baseline       *BaselinePolicyConfig
	RBACMiddleware *middleware.RBACMiddleware
}

type regimeBridgeAdapter struct {
	svc *service.PlatformRegimeService
}

func (a *regimeBridgeAdapter) EvaluateWorkloadCompliance(ctx context.Context, tenantID string, input WorkloadComplianceEvalInput) (bool, []WorkloadComplianceEvalViolation, error) {
	ok, violations, err := a.svc.EvaluateWorkloadCompliance(ctx, tenantID, contracts.WorkloadComplianceInput{
		Namespace:           input.Namespace,
		AuditRetentionDays:  input.AuditRetentionDays,
		HasEncryptedSecrets: input.HasEncryptedSecrets,
		KMSKeyURI:           input.KMSKeyURI,
	})
	if err != nil {
		return false, nil, err
	}
	out := make([]WorkloadComplianceEvalViolation, 0, len(violations))
	for _, v := range violations {
		out = append(out, WorkloadComplianceEvalViolation{
			Rule:    v.Rule,
			Message: v.Message,
		})
	}
	return ok, out, nil
}

type PlatformRegimeModule struct {
	svc     *service.PlatformRegimeService
	bridge  RegimeBridgeProvider
	handler *restv1.PlatformRegimeHandler
	router  *restv1.PlatformRegimeRouter
}

func BuildModule(deps ModuleDeps) *PlatformRegimeModule {
	cfg := DefaultBaselinePolicyConfig()
	if deps.Baseline != nil {
		cfg = *deps.Baseline
	}
	baseline := contracts.PlatformRegimePolicyRecord{
		ID:                   cfg.ID,
		Jurisdiction:         cfg.Jurisdiction,
		AuditRetentionDays:   cfg.AuditRetentionDays,
		DataResidencyRegions: cfg.DataResidencyRegions,
		KMSKeyCustodyMode:    contracts.KMSCustodyMode(cfg.KMSKeyCustodyMode),
		RTOMinutesTier0:      cfg.RTOMinutesTier0,
		RPOMinutesTier0:      cfg.RPOMinutesTier0,
		RTOMinutesTier1:      cfg.RTOMinutesTier1,
		RPOMinutesTier1:      cfg.RPOMinutesTier1,
		ClusterMode:          contracts.ClusterMode(cfg.ClusterMode),
		PrimaryRegionCount:   cfg.PrimaryRegionCount,
		StandbyRegionCount:   cfg.StandbyRegionCount,
		TenantIsolationMode:  contracts.TenancyMode(cfg.TenantIsolationMode),
		Status:               contracts.RegimePolicyStatus(cfg.Status),
	}
	repo := adapters.NewPostgresPlatformRegimeRepositoryAdapter(deps.DB)
	svc := service.NewPlatformRegimeService(repo, baseline)
	handler := restv1.NewPlatformRegimeHandler(svc)
	router := restv1.NewPlatformRegimeRouter(handler, deps.RBACMiddleware)
	return &PlatformRegimeModule{
		svc:     svc,
		bridge:  &regimeBridgeAdapter{svc: svc},
		handler: handler,
		router:  router,
	}
}

func (m *PlatformRegimeModule) Name() string                         { return "platform-regime" }
func (m *PlatformRegimeModule) Routes() []kernel.Route               { return nil }
func (m *PlatformRegimeModule) EventHandlers() []kernel.EventHandler { return nil }
func (m *PlatformRegimeModule) Lifecycles() []kernel.Lifecycle       { return nil }
func (m *PlatformRegimeModule) Provides() []string                   { return nil }
func (m *PlatformRegimeModule) BridgeProvider() RegimeBridgeProvider { return m.bridge }

func (m *PlatformRegimeModule) RegisterRoutes(mux *http.ServeMux) {
	if m.router != nil {
		m.router.RegisterRoutes(mux)
	}
}
