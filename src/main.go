package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	cliHandlers "internal-developer-platform/src/api/cli/handlers"
	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/middleware"
	"internal-developer-platform/src/api/rest/v1/routers"
	disasterrecovery "internal-developer-platform/src/features/disaster-recovery"
	drAdapters "internal-developer-platform/src/features/disaster-recovery/infrastructure/adapters"
	gitopsreconciliation "internal-developer-platform/src/features/gitops-reconciliation"
	healthAdapters "internal-developer-platform/src/features/health-checks/infrastructure/adapters"
	healthService "internal-developer-platform/src/features/health-checks/service"
	kubernetesworkloadprovisioning "internal-developer-platform/src/features/kubernetes-workload-provisioning"
	workloadAdapters "internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/adapters"
	workloadPorts "internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/ports"
	platformregime "internal-developer-platform/src/features/platform-regime"
	regimeContracts "internal-developer-platform/src/features/platform-regime/contracts"
	policyAdapters "internal-developer-platform/src/features/policy-as-code/infrastructure/adapters"
	policyPorts "internal-developer-platform/src/features/policy-as-code/infrastructure/ports"
	policyService "internal-developer-platform/src/features/policy-as-code/service"
	rbacAdapters "internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
	catalogAdapters "internal-developer-platform/src/features/service-catalog/infrastructure/adapters"
	catalogPorts "internal-developer-platform/src/features/service-catalog/infrastructure/ports"
	catalogService "internal-developer-platform/src/features/service-catalog/service"
	"internal-developer-platform/src/infra/chassis"
	argocdClientPkg "internal-developer-platform/src/infra/clients/argocd"
	"internal-developer-platform/src/infra/clients/cloudresource"
	"internal-developer-platform/src/infra/clients/cluster"
	k8sClientPkg "internal-developer-platform/src/infra/clients/kubernetes"
	"internal-developer-platform/src/infra/clients/resilience"
)

type AppServices struct {
	RBACRouter      *routers.RBACRouter
	CatalogRouter   *routers.ServiceCatalogRouter
	GitOpsRouter    *routers.GitOpsReconciliationRouter
	WorkloadRouter  *routers.KubernetesWorkloadRouter
	RegimeRouter    *routers.PlatformRegimeRouter
	HealthRouter    *routers.HealthCheckRouter
	OfferingsRouter *routers.OfferingsRouter
	PolicyRouter    *routers.PolicyAsCodeRouter
	DRRouter        *routers.DisasterRecoveryRouter
	DRCLIHandler    *cliHandlers.DisasterRecoveryCLIHandler
	DRService       *disasterrecovery.Service
}

func main() {
	os.Exit(chassis.Run(describeService(), os.Args[1:], os.Environ()))
}

func describeService() chassis.ServiceSpec {
	return chassis.ServiceSpec{
		Identity: func() chassis.Identity {
			return chassis.Identity{
				Name:    "internal-developer-platform",
				Team:    "platform-core",
				Tier:    "tier-1",
				Version: "1.0.0",
			}
		},
		Needs: func() chassis.InfrastructureNeeds {
			return chassis.InfrastructureNeeds{
				Database: true,
				Redis:    true,
				Producer: false,
				Consumer: false,
				Flags:    false,
			}
		},
		DependsOn: func() []chassis.DownstreamDependency {
			return []chassis.DownstreamDependency{
				{Name: "kubernetes-api", Criticality: chassis.CriticalitySoft},
				{Name: "argocd-server", Criticality: chassis.CriticalitySoft},
			}
		},
		Contracts: func() chassis.ServiceContracts {
			return chassis.ServiceContracts{
				ApiSchemas:     []string{"contracts/openapi/v1.yaml"},
				ProducedTopics: nil,
				ConsumedTopics: nil,
			}
		},
		BuildServices: func(deps chassis.Dependencies) (interface{}, error) {
			revocationRepo := rbacAdapters.NewRedisRevocationAdapter(deps.Config.Redis.Address, time.Duration(deps.Config.Redis.TimeoutSeconds)*time.Second)
			tokenVerifier := rbacAdapters.NewJWTTokenVerifierAdapter(deps.Config.Auth.JWTSecret, deps.Config.Auth.Issuer)
			rbacSvc := rbacService.NewRBACService(tokenVerifier, revocationRepo)
			rbacMW := middleware.NewRBACMiddleware(rbacSvc)
			rbacHandler := handlers.NewRBACHandler(rbacSvc)
			rbacRouter := routers.NewRBACRouter(rbacHandler, rbacMW)

			var catalogRepo catalogPorts.ServiceCatalogRepositoryPort
			var gitopsStore gitopsreconciliation.StorePort
			var workloadStore kubernetesworkloadprovisioning.StorePort
			var regimeRepo platformregime.PlatformRegimeRepositoryPort
			var policyRepo policyPorts.PolicyRepositoryPort
			var drStore disasterrecovery.DRPlanStorePort

			if deps.DB != nil {
				rbacPostgres := rbacAdapters.NewPostgresRBACAdapter(deps.DB)
				rbacSvc.SetGrantRepository(rbacPostgres)
				rbacSvc.SetUserBindingRepository(rbacPostgres)
				rbacSvc.SetAuditOutbox(rbacAdapters.NewPostgresRBACAuditOutboxAdapter(deps.DB))
				catalogRepo = catalogAdapters.NewPostgresServiceCatalogAdapter(deps.DB)
				gitopsStore = gitopsreconciliation.NewPostgresStore(deps.DB)
				workloadStore = workloadAdapters.NewPostgresWorkloadRepositoryAdapter(deps.DB)
				regimeRepo = platformregime.NewPostgresRepository(deps.DB)
				policyRepo = policyAdapters.NewPostgresPolicyRepositoryAdapter(deps.DB)
				drStore = drAdapters.NewPostgresDRRepositoryAdapter(deps.DB)
			} else {
				rbacMemGrants := rbacAdapters.NewMemoryAccessGrantAdapter()
				rbacMemBindings := rbacAdapters.NewMemoryUserBindingAdapter()
				rbacSvc.SetGrantRepository(rbacMemGrants)
				rbacSvc.SetUserBindingRepository(rbacMemBindings)
				rbacSvc.SetAuditOutbox(rbacAdapters.NewMemoryRBACAuditOutboxAdapter())
				catalogRepo = catalogAdapters.NewMemoryServiceCatalogAdapter()
				gitopsStore = gitopsreconciliation.NewMemoryStore()
				workloadStore = workloadAdapters.NewMemoryWorkloadRepositoryAdapter()
				regimeRepo = platformregime.NewMemoryRepository()
				policyRepo = policyAdapters.NewMemoryPolicyRepositoryAdapter()
				drStore = drAdapters.NewMemoryDRStoreAdapter()
			}

			catalogSvc := catalogService.NewServiceCatalogService(catalogRepo)
			catalogHandler := handlers.NewServiceCatalogHandler(catalogSvc)
			catalogRouter := routers.NewServiceCatalogRouter(catalogHandler, rbacMW)

			resConfig := resilience.DefaultResilienceConfig()
			var argoClient gitopsreconciliation.ClientPort
			if deps.Config.ArgoCD.Endpoint != "" && deps.Config.ArgoCD.Token != "" {
				liveArgo := argocdClientPkg.NewArgoCDClient(argocdClientPkg.ArgoCDClientConfig{
					BaseURL:   deps.Config.ArgoCD.Endpoint,
					AuthToken: deps.Config.ArgoCD.Token,
					Timeout:   15 * time.Second,
				}, resConfig)
				argoClient = gitopsreconciliation.NewLiveArgoCDClient(liveArgo)
			} else {
				argoClient = gitopsreconciliation.NewArgoCDClient(deps.Config.ArgoCD.Endpoint, deps.Config.ArgoCD.Token)
			}
			gitopsSvc := gitopsreconciliation.NewService(gitopsStore, argoClient)
			gitopsSvc.SetSecretAdapter(gitopsreconciliation.NewSecretAdapterWithDB(deps.DB))
			gitopsSvc.SetOutboxPort(gitopsreconciliation.NewOutboxAdapterWithDB(deps.DB))
			if deps.DB != nil {
				go func(relayCtx context.Context) {
					ticker := time.NewTicker(5 * time.Second)
					defer ticker.Stop()
					for {
						select {
						case <-relayCtx.Done():
							return
						case <-ticker.C:
							_, _ = gitopsSvc.RelayPendingOutboxEvents(relayCtx, 50, func(ctx context.Context, ev gitopsreconciliation.OutboxEvent) error {
								return nil
							})
						}
					}
				}(context.Background())
			}
			gitopsHandler := handlers.NewGitOpsReconciliationHandler(gitopsSvc)
			gitopsRouter := routers.NewGitOpsReconciliationRouter(gitopsHandler, rbacMW)

			var k8sClient workloadPorts.K8sClientPort
			if realK8s, err := k8sClientPkg.NewKubernetesClientFromConfig("", deps.Config.Kubernetes.Endpoint, resConfig); err == nil && realK8s != nil {
				k8sClient = workloadAdapters.NewLiveKubernetesClientAdapter(realK8s)
			} else {
				k8sClient = workloadAdapters.NewKubernetesClientAdapter(deps.Config.Kubernetes.Endpoint)
			}
			workloadSvc := kubernetesworkloadprovisioning.NewService(workloadStore, k8sClient)
			workloadHandler := handlers.NewKubernetesWorkloadHandler(workloadSvc)
			workloadRouter := routers.NewKubernetesWorkloadRouter(workloadHandler, rbacMW)

			healthRepo := healthAdapters.NewMemoryHealthCheckerAdapter()
			healthSvc := healthService.NewHealthCheckService(healthRepo)
			healthHandler := handlers.NewHealthCheckHandler(healthSvc)
			healthRouter := routers.NewHealthCheckRouter(healthHandler)

			regimeBaseline := regimeContracts.PlatformRegimePolicyRecord{
				ID:                   "baseline-default",
				Jurisdiction:         "US_FEDRAMP_MODERATE",
				AuditRetentionDays:   365,
				DataResidencyRegions: []string{"us-central1", "us-east4"},
				KMSKeyCustodyMode:    regimeContracts.KMSCustodyCMEK,
				RTOMinutesTier0:      15,
				RPOMinutesTier0:      0,
				RTOMinutesTier1:      60,
				RPOMinutesTier1:      15,
				ClusterMode:          regimeContracts.ClusterModeGKEStandard,
				PrimaryRegionCount:   2,
				StandbyRegionCount:   1,
				TenantIsolationMode:  regimeContracts.TenancyModeInternalOnly,
				Status:               regimeContracts.RegimeStatusActive,
			}

			regimeSvc := platformregime.NewService(regimeRepo, regimeBaseline)
			regimeHandler := handlers.NewPlatformRegimeHandler(regimeSvc)
			regimeRouter := routers.NewPlatformRegimeRouter(regimeHandler, rbacMW)

			workloadSvc.WithRegimeRepository(regimeRepo)

			clusterClient := cluster.NewGKEClusterClient(resConfig)
			clusterHandler := handlers.NewClusterHandler(clusterClient)

			cloudResourceClient := cloudresource.NewCloudResourceClient(resConfig)
			cloudResourceHandler := handlers.NewCloudResourceHandler(cloudResourceClient)

			offeringsRouter := routers.NewOfferingsRouter(clusterHandler, cloudResourceHandler, rbacMW)

			policySvc := policyService.NewPolicyAsCodeService(policyRepo)
			policyHandler := handlers.NewPolicyAsCodeHandler(policySvc)
			policyRouter := routers.NewPolicyAsCodeRouter(policyHandler, rbacMW)

			drCloud := drAdapters.NewCloudProviderDRAdapter()
			drSvc := disasterrecovery.NewService(drStore, drCloud, drCloud, drCloud, drCloud, drCloud)
			drHandler := handlers.NewDisasterRecoveryHandler(drSvc)
			drRouter := routers.NewDisasterRecoveryRouter(drHandler, rbacMW)
			drCLI := cliHandlers.NewDisasterRecoveryCLIHandler()

			return &AppServices{
				RBACRouter:      rbacRouter,
				CatalogRouter:   catalogRouter,
				GitOpsRouter:    gitopsRouter,
				WorkloadRouter:  workloadRouter,
				RegimeRouter:    regimeRouter,
				HealthRouter:    healthRouter,
				OfferingsRouter: offeringsRouter,
				PolicyRouter:    policyRouter,
				DRRouter:        drRouter,
				DRCLIHandler:    drCLI,
				DRService:       drSvc,
			}, nil
		},
		RegisterApiRoutes: func(mux *http.ServeMux, services interface{}) {
			appServices := services.(*AppServices)
			appServices.RBACRouter.RegisterRoutes(mux)
			appServices.CatalogRouter.RegisterRoutes(mux)
			appServices.GitOpsRouter.RegisterRoutes(mux)
			appServices.WorkloadRouter.RegisterRoutes(mux)
			appServices.RegimeRouter.RegisterRoutes(mux)
			appServices.OfferingsRouter.RegisterRoutes(mux)
			appServices.PolicyRouter.RegisterRoutes(mux)
			appServices.DRRouter.RegisterRoutes(mux)
			appServices.HealthRouter.RegisterPublicRoutes(mux)
		},

		RegisterAdminRoutes: func(mux *http.ServeMux, services interface{}) {
			appServices := services.(*AppServices)
			appServices.HealthRouter.RegisterAdminRoutes(mux)
		},

		StartBackgroundWorkers: func(ctx context.Context, services interface{}) {
			appServices := services.(*AppServices)
			if appServices.DRService != nil {
				appServices.DRService.StartBackgroundProber(ctx, 30*time.Second)
			}
		},

		ExecuteCommand: func(ctx context.Context, command string, args []string, services interface{}) (int, bool) {
			appServices := services.(*AppServices)
			switch command {
			case "dr-ctl", "dr":
				return appServices.DRCLIHandler.Execute(ctx, args), true
			default:
				fmt.Fprintf(os.Stderr, "unknown operational command: %s\n", command)
				return 2, true
			}
		},
	}
}
