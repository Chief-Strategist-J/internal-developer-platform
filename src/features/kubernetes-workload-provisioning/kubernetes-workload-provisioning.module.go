// Package kubernetesworkloadprovisioning: kubernetes-workload-provisioning.module assembles
// the Kubernetes Workload Provisioning feature module (V.3 role 15, X-05).
//
// Blueprint:
//  1. Constructs Postgres store, Kubernetes client, service, handler, and router.
//  2. Consumes RBAC async-job-drift capability indirectly via WithAsyncDriftPort (X-03).
//
// Invariants:
//   - No raw goroutines or timers spawned during BuildModule (G-11).
package kubernetesworkloadprovisioning

import (
	"database/sql"
	"net/http"

	restv1 "internal-developer-platform/src/features/kubernetes-workload-provisioning/api/rest/v1"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/adapters"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/ports"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/service"
	"internal-developer-platform/src/infra/clients/kubernetes"
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
	AsyncDriftPort ports.AsyncJobDriftPort
}

type WorkloadModule struct {
	svc     *service.WorkloadProvisioningService
	handler *restv1.KubernetesWorkloadHandler
	router  *restv1.KubernetesWorkloadRouter
}

func BuildModule(deps ModuleDeps) *WorkloadModule {
	store := adapters.NewPostgresWorkloadRepositoryAdapter(deps.DB)
	var k8sClient ports.K8sClientPort
	if realK8s, err := kubernetes.NewKubernetesClientFromConfig("", deps.Config.Kubernetes.Endpoint, deps.ResilienceCfg); err == nil && realK8s != nil {
		k8sClient = adapters.NewLiveKubernetesClientAdapter(realK8s)
	} else {
		k8sClient = adapters.NewKubernetesClientAdapter(deps.Config.Kubernetes.Endpoint)
	}
	svc := service.NewWorkloadProvisioningService(store, k8sClient)
	if deps.AsyncDriftPort != nil {
		svc.WithAsyncDriftPort(deps.AsyncDriftPort)
	}
	handler := restv1.NewKubernetesWorkloadHandler(svc)
	router := restv1.NewKubernetesWorkloadRouter(handler, deps.RBACMiddleware)
	return &WorkloadModule{
		svc:     svc,
		handler: handler,
		router:  router,
	}
}

func (m *WorkloadModule) Name() string                                  { return "kubernetes-workload-provisioning" }
func (m *WorkloadModule) Routes() []kernel.Route                        { return nil }
func (m *WorkloadModule) EventHandlers() []kernel.EventHandler          { return nil }
func (m *WorkloadModule) Lifecycles() []kernel.Lifecycle                { return nil }
func (m *WorkloadModule) Provides() []string                            { return nil }
func (m *WorkloadModule) Service() *service.WorkloadProvisioningService { return m.svc }

func (m *WorkloadModule) RegisterRoutes(mux *http.ServeMux) {
	if m.router != nil {
		m.router.RegisterRoutes(mux)
	}
}
