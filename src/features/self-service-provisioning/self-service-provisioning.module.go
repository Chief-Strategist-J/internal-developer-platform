/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING MODULE (V.3 role 15, X-05)

 1. Role:
    - Assembles cluster & cloud resource client adapters, service, REST handler, router, and sweeper lifecycle.
 2. Invariants:
    - Zero goroutines or tickers in BuildModule (G-11); does not export Service (X-05).
    - Zero inline comments below this header.
*/
package selfserviceprovisioning

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	restv1 "internal-developer-platform/src/features/self-service-provisioning/internal/api/rest/v1"
	"internal-developer-platform/src/features/self-service-provisioning/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/self-service-provisioning/internal/infrastructure/lifecycle"
	"internal-developer-platform/src/features/self-service-provisioning/internal/service"
	"internal-developer-platform/src/infra/clients/cloudresource"
	"internal-developer-platform/src/infra/clients/cluster"
	"internal-developer-platform/src/infra/clients/resilience"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/kernel"
)

type ModuleDeps struct {
	DB                    *sql.DB
	RedisAddr             string
	ResilienceCfg         resilience.ResilienceConfig
	RBACMiddleware        *middleware.RBACMiddleware
	IdempotencyMiddleware *middleware.IdempotencyMiddleware
}

type SelfServiceProvisioningModule struct {
	router     *restv1.SelfServiceProvisioningRouter
	routes     []kernel.Route
	lifecycles []kernel.Lifecycle
}

func BuildModule(deps ModuleDeps) *SelfServiceProvisioningModule {
	clusterClient := cluster.NewGKEClusterClient(deps.ResilienceCfg)
	cloudResourceClient := cloudresource.NewCloudResourceClient(deps.ResilienceCfg)
	clusterAdapter := adapters.NewGKEClusterClientAdapter(clusterClient)
	cloudResourceAdapter := adapters.NewCloudResourceClientAdapter(cloudResourceClient)
	svc := service.NewSelfServiceProvisioningService(clusterAdapter, cloudResourceAdapter)
	if deps.DB != nil {
		svc.SetLedgerPort(adapters.NewPostgresProvisioningLedgerAdapter(deps.DB))
		svc.SetReservationPort(adapters.NewPostgresProvisioningReservationAdapter(deps.DB))
		svc.SetApprovalPort(adapters.NewPostgresProvisioningApprovalAdapter(deps.DB))
		svc.SetGovernancePort(adapters.NewPostgresProvisioningGovernanceAdapter(deps.DB))
	} else {
		svc.SetLedgerPort(adapters.NewMemoryProvisioningLedgerAdapter())
		svc.SetReservationPort(adapters.NewMemoryProvisioningReservationAdapter())
		svc.SetApprovalPort(adapters.NewMemoryProvisioningApprovalAdapter())
		svc.SetGovernancePort(adapters.NewMemoryProvisioningGovernanceAdapter())
	}
	handler := restv1.NewSelfServiceProvisioningHandler(svc)
	router := restv1.NewSelfServiceProvisioningRouter(handler, deps.RBACMiddleware, deps.IdempotencyMiddleware)

	var lcs []kernel.Lifecycle
	var redisClient *redis.Client
	if deps.RedisAddr != "" {
		redisClient = redis.NewClient(&redis.Options{
			Addr:        deps.RedisAddr,
			DialTimeout: 2 * time.Second,
		})
	}
	if deps.DB != nil {
		lcs = append(lcs, lifecycle.NewSelfServiceSweeperLifecycle(svc, redisClient, 24*time.Hour))
	}

	return &SelfServiceProvisioningModule{
		router:     router,
		lifecycles: lcs,
	}
}

func (m *SelfServiceProvisioningModule) Name() string                         { return "self-service-provisioning" }
func (m *SelfServiceProvisioningModule) Routes() []kernel.Route               { return m.routes }
func (m *SelfServiceProvisioningModule) EventHandlers() []kernel.EventHandler { return nil }
func (m *SelfServiceProvisioningModule) Lifecycles() []kernel.Lifecycle       { return m.lifecycles }
func (m *SelfServiceProvisioningModule) Provides() []string                   { return nil }

func (m *SelfServiceProvisioningModule) RegisterRoutes(mux *http.ServeMux) {
	if m.router != nil {
		m.router.RegisterRoutes(mux)
	}
}
