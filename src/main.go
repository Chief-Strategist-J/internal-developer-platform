// Package main is the composition root for internal-developer-platform (L-02, C.6).
//
// Blueprint:
//  1. Phase load-config / init-observability / build-infra handled by chassis.Run.
//  2. Phase build-modules constructs feature modules in dependency order (providers first).
//  3. Phase bind-capabilities & bind-bridges wires kernel Authorizer and cross-feature bridges.
//  4. Phase start-lifecycles starts all module Lifecycle runners from StartBackgroundWorkers (G-11, P3-1).
//
// Invariants:
//   - BuildServices contains ZERO goroutine spawns or time.NewTicker calls (G-11).
//   - Zero mid-function inline comments.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	cliHandlers "internal-developer-platform/src/api/cli/handlers"
	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/routers"
	"internal-developer-platform/src/app"
	"internal-developer-platform/src/app/bridges"
	disasterrecovery "internal-developer-platform/src/features/disaster-recovery"
	drAdapters "internal-developer-platform/src/features/disaster-recovery/infrastructure/adapters"
	gitopsreconciliation "internal-developer-platform/src/features/gitops-reconciliation"
	healthAdapters "internal-developer-platform/src/features/health-checks/infrastructure/adapters"
	healthService "internal-developer-platform/src/features/health-checks/service"
	kubernetesworkloadprovisioning "internal-developer-platform/src/features/kubernetes-workload-provisioning"
	platformregime "internal-developer-platform/src/features/platform-regime"
	regimeContracts "internal-developer-platform/src/features/platform-regime/contracts"
	policyAdapters "internal-developer-platform/src/features/policy-as-code/infrastructure/adapters"
	policyService "internal-developer-platform/src/features/policy-as-code/service"
	rbac "internal-developer-platform/src/features/role-based-access-control"
	catalogAdapters "internal-developer-platform/src/features/service-catalog/infrastructure/adapters"
	catalogService "internal-developer-platform/src/features/service-catalog/service"
	"internal-developer-platform/src/infra/chassis"
	"internal-developer-platform/src/infra/clients/cloudresource"
	"internal-developer-platform/src/infra/clients/cluster"
	"internal-developer-platform/src/infra/clients/resilience"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/kernel"
)

type AppServices struct {
	Registry        *app.Registry
	Bindings        app.CapabilityBindings
	RBACModule      *rbac.RBACModule
	GitOpsModule    *gitopsreconciliation.GitOpsModule
	WorkloadModule  *kubernetesworkloadprovisioning.WorkloadModule
	CatalogRouter   *routers.ServiceCatalogRouter
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
			if deps.DB == nil {
				return nil, fmt.Errorf("database connection is required for production persistence")
			}

			resConfig := resilience.DefaultResilienceConfig()
			rbacMod := rbac.BuildModule(rbac.ModuleDeps{
				DB:     deps.DB,
				Config: deps.Config,
			})
			bindings := app.CapabilityBindings{
				Authorizer: rbacMod.Authorizer(),
			}
			rbacMW := middleware.NewRBACMiddleware(bindings.Authorizer)

			gitopsBridge := bridges.NewGitopsRBACBridge(rbacMod.Service())
			workloadBridge := bridges.NewWorkloadRBACBridge(rbacMod.Service())

			gitopsMod := gitopsreconciliation.BuildModule(gitopsreconciliation.ModuleDeps{
				DB:             deps.DB,
				Config:         deps.Config,
				ResilienceCfg:  resConfig,
				RBACMiddleware: rbacMW,
				TaskLeasePort:  gitopsBridge,
			})
			workloadMod := kubernetesworkloadprovisioning.BuildModule(kubernetesworkloadprovisioning.ModuleDeps{
				DB:             deps.DB,
				Config:         deps.Config,
				ResilienceCfg:  resConfig,
				RBACMiddleware: rbacMW,
				AsyncDriftPort: workloadBridge,
			})

			catalogRepo := catalogAdapters.NewPostgresServiceCatalogAdapter(deps.DB)
			regimeRepo := platformregime.NewPostgresRepository(deps.DB)
			policyRepo := policyAdapters.NewPostgresPolicyRepositoryAdapter(deps.DB)
			drStore := drAdapters.NewPostgresDRRepositoryAdapter(deps.DB)

			catalogSvc := catalogService.NewServiceCatalogService(catalogRepo)
			catalogHandler := handlers.NewServiceCatalogHandler(catalogSvc)
			catalogRouter := routers.NewServiceCatalogRouter(catalogHandler, rbacMW)

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

			workloadMod.Service().WithRegimeRepository(regimeRepo)

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

			registry := app.NewRegistry(rbacMod, gitopsMod, workloadMod)

			return &AppServices{
				Registry:        registry,
				Bindings:        bindings,
				RBACModule:      rbacMod,
				GitOpsModule:    gitopsMod,
				WorkloadModule:  workloadMod,
				CatalogRouter:   catalogRouter,
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
			appServices.RBACModule.RegisterRoutes(mux)
			appServices.GitOpsModule.RegisterRoutes(mux)
			appServices.WorkloadModule.RegisterRoutes(mux)
			appServices.CatalogRouter.RegisterRoutes(mux)
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
			if appServices.Registry != nil {
				for _, lc := range appServices.Registry.Lifecycles() {
					startLifecycle(ctx, lc)
				}
			}
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

func startLifecycle(ctx context.Context, lc kernel.Lifecycle) {
	if lc == nil {
		return
	}
	_ = lc.Start(ctx)
}
