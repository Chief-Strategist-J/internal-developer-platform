// Package rbac: role-based-access-control.module assembles the RBAC feature from injected
// infrastructure and returns its public Module surface (V.3 role 15, X-05).
//
// Blueprint:
//  1. Assembles service, router, authorizer capability, bridge adapter, and lifecycles.
//  2. Provides the kernel authz.Authorizer capability via RBACAuthorizerProvider (X-02).
//  3. Lifecycles (prune-scheduler, streaming-reaper, audit-relay) are returned rather than started here (G-11).
//
// Invariants:
//   - No business logic here — only construction and wiring.
//   - Never exports *service.RBACService (X-05, GO-INT-001).
package rbac

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	restv1 "internal-developer-platform/src/features/role-based-access-control/internal/api/rest/v1"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/internal/contracts"
	"internal-developer-platform/src/features/role-based-access-control/internal/infrastructure/adapters"
	rbacLifecycle "internal-developer-platform/src/features/role-based-access-control/internal/infrastructure/lifecycle"
	"internal-developer-platform/src/features/role-based-access-control/internal/service"
	"internal-developer-platform/src/infra/config"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/kernel"
	"internal-developer-platform/src/shared/kernel/capabilities/authz"
)

type ModuleDeps struct {
	DB     *sql.DB
	Config *config.AppConfig
}

type rbacBridgeAdapter struct {
	svc *service.RBACService
}

func (a *rbacBridgeAdapter) EvaluateTaskLease(ctx context.Context, req TaskLeaseEvalRequest) (TaskLeaseEvalResult, error) {
	res, err := a.svc.EvaluateTaskLease(ctx, rbacContracts.TaskExecutionLeaseRequest{
		TokenExpired:          req.TokenExpired,
		HasSignedTaskLease:    req.HasSignedTaskLease,
		LeaseExpired:          req.LeaseExpired,
		IsIdempotentBatchStep: req.IsIdempotentBatchStep,
		RefreshSucceeded:      req.RefreshSucceeded,
	})
	return TaskLeaseEvalResult{
		Granted:       res.Granted,
		Reason:        res.Reason,
		Directive:     res.Directive,
		ViolationCode: res.ViolationCode,
	}, err
}

func (a *rbacBridgeAdapter) EvaluateAsyncJobDrift(ctx context.Context, subjectID string, jti string, queueLatency time.Duration) (AsyncJobDriftEvalResult, error) {
	res, err := a.svc.EvaluateAsyncJobDrift(ctx, subjectID, jti, queueLatency, nil)
	return AsyncJobDriftEvalResult{
		Granted:       res.Granted,
		Reason:        res.Reason,
		Directive:     res.Directive,
		ViolationCode: res.ViolationCode,
	}, err
}

type RBACModule struct {
	svc        *service.RBACService
	authorizer authz.Authorizer
	bridge     RBACBridgeProvider
	handler    *restv1.RBACHandler
	router     *restv1.RBACRouter
	routes     []kernel.Route
	lifecycles []kernel.Lifecycle
}

func BuildModule(deps ModuleDeps) *RBACModule {
	cfg := deps.Config
	var revocationRepo *adapters.RedisRevocationAdapter
	if len(cfg.Redis.SentinelAddrs) > 0 && cfg.Redis.SentinelMaster != "" {
		revocationRepo = adapters.NewRedisSentinelRevocationAdapter(
			cfg.Redis.SentinelAddrs,
			cfg.Redis.SentinelMaster,
			time.Duration(cfg.Redis.TimeoutSeconds)*time.Second,
		)
	} else {
		revocationRepo = adapters.NewRedisRevocationAdapter(
			cfg.Redis.Address,
			time.Duration(cfg.Redis.TimeoutSeconds)*time.Second,
		)
	}
	tokenVerifier := adapters.NewJWTTokenVerifierAdapter(cfg.Auth.JWTSecret, cfg.Auth.Issuer)
	rbacPostgres := adapters.NewPostgresRBACAdapter(deps.DB)
	rbacAuditOutbox := adapters.NewPostgresRBACAuditOutboxAdapter(deps.DB)
	ticketVerifier := adapters.NewHTTPBreakGlassTicketAdapter(
		cfg.RBAC.PagerDutyEndpoint,
		cfg.RBAC.PagerDutyAPIToken,
		cfg.RBAC.JiraEndpoint,
		cfg.RBAC.JiraAPIToken,
	)
	siemEmitter := adapters.NewHTTPSIEMAlertAdapter(
		cfg.RBAC.SIEMWebhookURL,
		cfg.RBAC.SIEMAPIToken,
	)
	oidcIntrospector := adapters.NewHTTPOIDCIntrospectorAdapter(
		cfg.RBAC.OIDCIntrospectEndpoint,
		cfg.RBAC.OIDCClientID,
		cfg.RBAC.OIDCClientSecret,
	)
	svc := service.NewRBACServiceWithDeps(service.RBACServiceDeps{
		TokenVerifier:        tokenVerifier,
		RevocationStore:      revocationRepo,
		GrantRepo:            rbacPostgres,
		BindingRepo:          rbacPostgres,
		AuditOutbox:          rbacAuditOutbox,
		UnitOfWork:           rbacPostgres,
		TicketVerifier:       ticketVerifier,
		SIEMAlertEmitter:     siemEmitter,
		RoleDefRepo:          rbacPostgres,
		OIDCIntrospector:     oidcIntrospector,
		ExclusiveClaimRepo:   rbacPostgres,
		FederationHMACSecret: cfg.RBAC.FederationHMACSecret,
		TaskLeaseHMACSecret:  cfg.RBAC.TaskLeaseHMACSecret,
		BootstrapSecret:      cfg.RBAC.BootstrapSecret,
		Environment:          cfg.Environment,
		DevStagingMaxQuota:   cfg.RBAC.DevStagingMaxQuota,
		AllowedStartHourUTC:  cfg.RBAC.ABACAllowedStartHour,
		AllowedEndHourUTC:    cfg.RBAC.ABACAllowedEndHour,
	})
	if cfg.RBAC.SidecarPDPEndpoint != "" {
		sidecarAdapter := adapters.NewHTTPSidecarPDPAdapter(rbacContracts.SidecarPDPConfig{
			Engine:    rbacContracts.SidecarPDPEngine(cfg.RBAC.SidecarPDPEngine),
			Endpoint:  cfg.RBAC.SidecarPDPEndpoint,
			TimeoutMs: cfg.RBAC.SidecarPDPTimeoutMs,
		})
		svc.SetSidecarPDP(sidecarAdapter)
	}
	authorizer := service.NewRBACAuthorizerProvider(svc)
	mw := middleware.NewRBACMiddleware(authorizer)
	handler := restv1.NewRBACHandler(svc)
	router := restv1.NewRBACRouter(handler, mw)
	lifecycles := []kernel.Lifecycle{
		rbacLifecycle.NewInvalidationListenerLifecycle(revocationRepo, svc.InvalidateLocalCache),
		rbacLifecycle.NewPruneSchedulerLifecycle(svc, revocationRepo, 24*time.Hour),
		rbacLifecycle.NewStreamingReaperLifecycle(svc, 60*time.Second),
		rbacLifecycle.NewAuditOutboxRelayLifecycle(rbacAuditOutbox, siemEmitter, 5*time.Second),
	}
	return &RBACModule{
		svc:        svc,
		authorizer: authorizer,
		bridge:     &rbacBridgeAdapter{svc: svc},
		handler:    handler,
		router:     router,
		lifecycles: lifecycles,
	}
}

func (m *RBACModule) Name() string { return "role-based-access-control" }

func (m *RBACModule) Routes() []kernel.Route { return m.routes }

func (m *RBACModule) RegisterRoutes(mux *http.ServeMux) {
	if m.router != nil {
		m.router.RegisterRoutes(mux)
	}
}

func (m *RBACModule) EventHandlers() []kernel.EventHandler { return nil }

func (m *RBACModule) Lifecycles() []kernel.Lifecycle { return m.lifecycles }

func (m *RBACModule) Provides() []string { return []string{"authz"} }

func (m *RBACModule) Authorizer() authz.Authorizer { return m.authorizer }

func (m *RBACModule) BridgeProvider() RBACBridgeProvider { return m.bridge }
