/*
ALGORITHM & ARCHITECTURE BLUEPRINT: COMPOSITION ROOT MAIN ENTRYPOINT (L-02, C.6)

 1. Role:
    - Constructs all 9 active feature modules via their BuildModule constructors in dependency order.
    - Binds the kernel authz.Authorizer capability and wires all cross-feature X-03 bridges.
    - Starts every registered kernel.Lifecycle from StartBackgroundWorkers (G-11, P3-1).
 2. Invariants:
    - Zero goroutine spawns or time.NewTicker calls in BuildServices (G-11).
    - Zero comments below this header.
*/
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"internal-developer-platform/src/app"
	"internal-developer-platform/src/app/bridges"
	disasterrecovery "internal-developer-platform/src/features/disaster-recovery"
	gitopsreconciliation "internal-developer-platform/src/features/gitops-reconciliation"
	healthchecks "internal-developer-platform/src/features/health-checks"
	imagesecurity "internal-developer-platform/src/features/image-security"
	kubernetesworkloadprovisioning "internal-developer-platform/src/features/kubernetes-workload-provisioning"
	platformregime "internal-developer-platform/src/features/platform-regime"
	policyascode "internal-developer-platform/src/features/policy-as-code"
	rbac "internal-developer-platform/src/features/role-based-access-control"
	selfserviceprovisioning "internal-developer-platform/src/features/self-service-provisioning"
	servicecatalog "internal-developer-platform/src/features/service-catalog"
	"internal-developer-platform/src/infra/chassis"
	"internal-developer-platform/src/infra/clients/resilience"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/kernel"
)

type AppServices struct {
	Registry          *app.Registry
	Bindings          app.CapabilityBindings
	RBACModule        *rbac.RBACModule
	GitOpsModule      *gitopsreconciliation.GitOpsModule
	WorkloadModule    *kubernetesworkloadprovisioning.WorkloadModule
	RegimeModule      *platformregime.PlatformRegimeModule
	PolicyModule      *policyascode.PolicyAsCodeModule
	CatalogModule     *servicecatalog.ServiceCatalogModule
	HealthModule      *healthchecks.HealthChecksModule
	DRModule          *disasterrecovery.DisasterRecoveryModule
	ImageSecModule    *imagesecurity.ImageSecurityModule
	SelfServiceModule *selfserviceprovisioning.SelfServiceProvisioningModule
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
			idempotencyMW := middleware.NewIdempotencyMiddlewareWithAddr(deps.Config.Redis.Address)

			regimeBaseline := platformregime.DefaultBaselinePolicyConfig()

			regimeMod := platformregime.BuildModule(platformregime.ModuleDeps{
				DB:             deps.DB,
				Baseline:       &regimeBaseline,
				RBACMiddleware: rbacMW,
			})
			policyMod := policyascode.BuildModule(policyascode.ModuleDeps{
				DB:             deps.DB,
				RBACMiddleware: rbacMW,
			})

			gitopsBridge := bridges.NewGitopsRBACBridge(rbacMod.BridgeProvider())
			workloadRBACBridge := bridges.NewWorkloadRBACBridge(rbacMod.BridgeProvider())
			workloadRegimeBridge := bridges.NewWorkloadRegimeBridge(regimeMod.BridgeProvider())
			workloadPolicyBridge := bridges.NewWorkloadPolicyBridge(policyMod.BridgeProvider())

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
				AsyncDriftPort: workloadRBACBridge,
				RegimeGuard:    workloadRegimeBridge,
				PolicyGuard:    workloadPolicyBridge,
			})
			catalogMod := servicecatalog.BuildModule(servicecatalog.ModuleDeps{
				DB:             deps.DB,
				RBACMiddleware: rbacMW,
			})
			healthMod := healthchecks.BuildModule()
			drMod := disasterrecovery.BuildModule(disasterrecovery.ModuleDeps{
				DB:             deps.DB,
				Config:         deps.Config,
				RBACMiddleware: rbacMW,
			})
			imageSecMod := imagesecurity.BuildModule(nil)

			selfServiceMod := selfserviceprovisioning.BuildModule(selfserviceprovisioning.ModuleDeps{
				DB:                    deps.DB,
				RedisAddr:             deps.Config.Redis.Address,
				ResilienceCfg:         resConfig,
				RBACMiddleware:        rbacMW,
				IdempotencyMiddleware: idempotencyMW,
			})

			registry := app.NewRegistry(
				rbacMod,
				regimeMod,
				policyMod,
				gitopsMod,
				workloadMod,
				catalogMod,
				healthMod,
				drMod,
				imageSecMod,
				selfServiceMod,
			)

			return &AppServices{
				Registry:          registry,
				Bindings:          bindings,
				RBACModule:        rbacMod,
				GitOpsModule:      gitopsMod,
				WorkloadModule:    workloadMod,
				RegimeModule:      regimeMod,
				PolicyModule:      policyMod,
				CatalogModule:     catalogMod,
				HealthModule:      healthMod,
				DRModule:          drMod,
				ImageSecModule:    imageSecMod,
				SelfServiceModule: selfServiceMod,
			}, nil
		},
		RegisterApiRoutes: func(mux *http.ServeMux, services interface{}) {
			appServices := services.(*AppServices)
			appServices.RBACModule.RegisterRoutes(mux)
			appServices.GitOpsModule.RegisterRoutes(mux)
			appServices.WorkloadModule.RegisterRoutes(mux)
			appServices.CatalogModule.RegisterRoutes(mux)
			appServices.RegimeModule.RegisterRoutes(mux)
			appServices.PolicyModule.RegisterRoutes(mux)
			appServices.DRModule.RegisterRoutes(mux)
			appServices.SelfServiceModule.RegisterRoutes(mux)
			appServices.HealthModule.RegisterPublicRoutes(mux)
		},
		RegisterAdminRoutes: func(mux *http.ServeMux, services interface{}) {
			appServices := services.(*AppServices)
			appServices.HealthModule.RegisterAdminRoutes(mux)
		},
		StartBackgroundWorkers: func(ctx context.Context, services interface{}) {
			appServices := services.(*AppServices)
			if appServices.Registry != nil {
				for _, lc := range appServices.Registry.Lifecycles() {
					startLifecycle(ctx, lc)
				}
			}
		},
		ExecuteCommand: func(ctx context.Context, command string, args []string, services interface{}) (int, bool) {
			appServices := services.(*AppServices)
			switch command {
			case "dr-ctl", "dr":
				return appServices.DRModule.ExecuteCLI(ctx, args), true
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
