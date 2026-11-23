package main

import (
	"net/http"
	"os"
	"time"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/middleware"
	"internal-developer-platform/src/api/rest/v1/routers"
	gitopsreconciliation "internal-developer-platform/src/features/gitops-reconciliation"
	healthAdapters "internal-developer-platform/src/features/health-checks/infrastructure/adapters"
	healthService "internal-developer-platform/src/features/health-checks/service"
	kubernetesworkloadprovisioning "internal-developer-platform/src/features/kubernetes-workload-provisioning"
	platformregime "internal-developer-platform/src/features/platform-regime"
	regimeContracts "internal-developer-platform/src/features/platform-regime/contracts"
	rbacAdapters "internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
	catalogAdapters "internal-developer-platform/src/features/service-catalog/infrastructure/adapters"
	catalogPorts "internal-developer-platform/src/features/service-catalog/infrastructure/ports"
	catalogService "internal-developer-platform/src/features/service-catalog/service"
	"internal-developer-platform/src/infra/chassis"
)

type AppServices struct {
	RBACRouter     *routers.RBACRouter
	CatalogRouter  *routers.ServiceCatalogRouter
	GitOpsRouter   *routers.GitOpsReconciliationRouter
	WorkloadRouter *routers.KubernetesWorkloadRouter
	RegimeRouter   *routers.PlatformRegimeRouter
	HealthRouter   *routers.HealthCheckRouter
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

			if deps.DB != nil {
				catalogRepo = catalogAdapters.NewPostgresServiceCatalogAdapter(deps.DB)
				gitopsStore = gitopsreconciliation.NewPostgresStore(deps.DB)
				workloadStore = kubernetesworkloadprovisioning.NewPostgresStore(deps.DB)
			} else {
				catalogRepo = catalogAdapters.NewMemoryServiceCatalogAdapter()
				gitopsStore = gitopsreconciliation.NewMemoryStore()
				workloadStore = kubernetesworkloadprovisioning.NewMemoryStore()
			}

			catalogSvc := catalogService.NewServiceCatalogService(catalogRepo)
			catalogHandler := handlers.NewServiceCatalogHandler(catalogSvc)
			catalogRouter := routers.NewServiceCatalogRouter(catalogHandler, rbacMW)

			argoClient := gitopsreconciliation.NewArgoCDClient(deps.Config.ArgoCD.Endpoint, deps.Config.ArgoCD.Token)
			gitopsSvc := gitopsreconciliation.NewService(gitopsStore, argoClient)
			gitopsHandler := handlers.NewGitOpsReconciliationHandler(gitopsSvc)
			gitopsRouter := routers.NewGitOpsReconciliationRouter(gitopsHandler, rbacMW)

			k8sClient := kubernetesworkloadprovisioning.NewK8sClient(deps.Config.Kubernetes.Endpoint)
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

			var regimeRepo platformregime.PlatformRegimeRepositoryPort
			if deps.DB != nil {
				regimeRepo = platformregime.NewPostgresRepository(deps.DB)
			} else {
				regimeRepo = platformregime.NewMemoryRepository()
			}
			regimeSvc := platformregime.NewService(regimeRepo, regimeBaseline)
			regimeHandler := handlers.NewPlatformRegimeHandler(regimeSvc)
			regimeRouter := routers.NewPlatformRegimeRouter(regimeHandler, rbacMW)

			workloadSvc.WithRegimeRepository(regimeRepo)

			return &AppServices{
				RBACRouter:     rbacRouter,
				CatalogRouter:  catalogRouter,
				GitOpsRouter:   gitopsRouter,
				WorkloadRouter: workloadRouter,
				RegimeRouter:   regimeRouter,
				HealthRouter:   healthRouter,
			}, nil
		},
		RegisterApiRoutes: func(mux *http.ServeMux, services interface{}) {
			appServices := services.(*AppServices)
			appServices.RBACRouter.RegisterRoutes(mux)
			appServices.CatalogRouter.RegisterRoutes(mux)
			appServices.GitOpsRouter.RegisterRoutes(mux)
			appServices.WorkloadRouter.RegisterRoutes(mux)
			appServices.RegimeRouter.RegisterRoutes(mux)
			appServices.HealthRouter.RegisterPublicRoutes(mux)
		},

		RegisterAdminRoutes: func(mux *http.ServeMux, services interface{}) {
			appServices := services.(*AppServices)
			appServices.HealthRouter.RegisterAdminRoutes(mux)
		},
	}
}
