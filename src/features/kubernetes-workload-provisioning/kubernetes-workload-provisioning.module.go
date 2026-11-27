// Package kubernetesworkloadprovisioning: kubernetes-workload-provisioning.module assembles
// the Kubernetes Workload Provisioning feature module (V.3 role 15, X-05).
//
// Blueprint:
//  1. Constructs Postgres store, Kubernetes client, service, handler, and router.
//  2. Consumes cross-feature governance capabilities via index bridge interfaces (X-03, GO-INT-001).
//
// Invariants:
//   - No raw goroutines or timers spawned during BuildModule (G-11).
//   - Never exports internal Service (X-05).
package kubernetesworkloadprovisioning

import (
	"context"
	"database/sql"
	"net/http"

	restv1 "internal-developer-platform/src/features/kubernetes-workload-provisioning/internal/api/rest/v1"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/internal/infrastructure/ports"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/internal/service"
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
	AsyncDriftPort AsyncDriftBridge
	RegimeGuard    RegimeGuardBridge
	PolicyGuard    PolicyExceptionBridge
}

type asyncDriftPortAdapter struct {
	bridge AsyncDriftBridge
}

func (a *asyncDriftPortAdapter) EvaluateAsyncJobDrift(ctx context.Context, req ports.AsyncJobDriftRequest) (ports.AsyncJobDriftResult, error) {
	res, err := a.bridge.EvaluateAsyncJobDrift(ctx, AsyncJobDriftRequest{
		SubjectID:    req.SubjectID,
		JTI:          req.JTI,
		QueueLatency: req.QueueLatency,
	})
	return ports.AsyncJobDriftResult{
		Revoked:   res.Revoked,
		Reason:    res.Reason,
		Granted:   res.Granted,
		Directive: res.Directive,
	}, err
}

type regimeGuardPortAdapter struct {
	bridge RegimeGuardBridge
}

func (a *regimeGuardPortAdapter) EvaluateWorkloadRegime(ctx context.Context, req ports.RegimeEvaluationRequest) (ports.RegimeEvaluationResult, error) {
	res, err := a.bridge.EvaluateWorkloadRegime(ctx, RegimeEvaluationRequest{
		TenantID:            req.TenantID,
		Namespace:           req.Namespace,
		AuditRetentionDays:  req.AuditRetentionDays,
		HasEncryptedSecrets: req.HasEncryptedSecrets,
		KMSKeyURI:           req.KMSKeyURI,
	})
	mapped := make([]ports.RegimeViolation, 0, len(res.Violations))
	for _, v := range res.Violations {
		mapped = append(mapped, ports.RegimeViolation{
			Rule:    v.Rule,
			Message: v.Message,
		})
	}
	return ports.RegimeEvaluationResult{
		Allowed:    res.Allowed,
		Violations: mapped,
	}, err
}

type policyGuardPortAdapter struct {
	bridge PolicyExceptionBridge
}

func (a *policyGuardPortAdapter) GetActiveExceptionForWorkload(ctx context.Context, tenantID string, namespace string, workloadName string) (*ports.ActivePolicyException, error) {
	exc, err := a.bridge.GetActiveExceptionForWorkload(ctx, tenantID, namespace, workloadName)
	if err != nil || exc == nil {
		return nil, err
	}
	return &ports.ActivePolicyException{
		PublicID:   exc.PublicID,
		ApproverID: exc.ApproverID,
	}, nil
}

type WorkloadModule struct {
	svc     *service.WorkloadProvisioningService
	handler *restv1.KubernetesWorkloadHandler
	router  *restv1.KubernetesWorkloadRouter
}

func BuildModule(deps ModuleDeps) *WorkloadModule {
	store := adapters.NewPostgresWorkloadRepositoryAdapter(deps.DB)
	var k8sClient ports.K8sClientPort
	if realK8s, err := kubernetes.NewKubernetesClientFromConfig(deps.Config.Kubernetes.KubeconfigPath, deps.Config.Kubernetes.Endpoint, deps.ResilienceCfg); err == nil && realK8s != nil {
		k8sClient = adapters.NewLiveKubernetesClientAdapter(realK8s)
	} else {
		k8sClient = adapters.NewKubernetesClientAdapter(deps.Config.Kubernetes.Endpoint)
	}
	svc := service.NewWorkloadProvisioningService(store, k8sClient)
	if deps.AsyncDriftPort != nil {
		svc.WithAsyncDriftPort(&asyncDriftPortAdapter{bridge: deps.AsyncDriftPort})
	}
	if deps.RegimeGuard != nil {
		svc.WithRegimeGuard(&regimeGuardPortAdapter{bridge: deps.RegimeGuard})
	}
	if deps.PolicyGuard != nil {
		svc.WithPolicyGuard(&policyGuardPortAdapter{bridge: deps.PolicyGuard})
	}
	handler := restv1.NewKubernetesWorkloadHandler(svc)
	router := restv1.NewKubernetesWorkloadRouter(handler, deps.RBACMiddleware)
	return &WorkloadModule{
		svc:     svc,
		handler: handler,
		router:  router,
	}
}

func (m *WorkloadModule) Name() string                         { return "kubernetes-workload-provisioning" }
func (m *WorkloadModule) Routes() []kernel.Route               { return nil }
func (m *WorkloadModule) EventHandlers() []kernel.EventHandler { return nil }
func (m *WorkloadModule) Lifecycles() []kernel.Lifecycle       { return nil }
func (m *WorkloadModule) Provides() []string                   { return nil }

func (m *WorkloadModule) RegisterRoutes(mux *http.ServeMux) {
	if m.router != nil {
		m.router.RegisterRoutes(mux)
	}
}
