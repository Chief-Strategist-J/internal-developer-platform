/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING REST ROUTER (V.3 role 01)

 1. Role:
    - Mounts cluster and cloud resource offering HTTP endpoints under self-service-provisioning.
 2. Invariants:
    - Zero inline comments; enforces RBAC middleware when wired.
*/
package restv1

import (
	"net/http"

	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/kernel/capabilities/authz"
)

type SelfServiceProvisioningRouter struct {
	handler     *SelfServiceProvisioningHandler
	rbacMiddle  *middleware.RBACMiddleware
	idempMiddle *middleware.IdempotencyMiddleware
}

func NewSelfServiceProvisioningRouter(handler *SelfServiceProvisioningHandler, rbacM *middleware.RBACMiddleware, idempM *middleware.IdempotencyMiddleware) *SelfServiceProvisioningRouter {
	return &SelfServiceProvisioningRouter{
		handler:     handler,
		rbacMiddle:  rbacM,
		idempMiddle: idempM,
	}
}

func (r *SelfServiceProvisioningRouter) RegisterRoutes(mux *http.ServeMux) {
	clHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			r.handler.ProvisionCluster(w, req)
		case http.MethodGet:
			r.handler.GetCluster(w, req)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	crHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.handler.ProvisionResource(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	postureHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			r.handler.ValidatePosture(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	reconcileHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.handler.ReconcileLedger(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	if r.idempMiddle != nil {
		clHandler = r.idempMiddle.Wrap(clHandler)
		crHandler = r.idempMiddle.Wrap(crHandler)
		reconcileHandler = r.idempMiddle.Wrap(reconcileHandler)
	}

	if r.rbacMiddle != nil {
		mux.HandleFunc(constants.EndpointClusters, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			clHandler,
		))
		mux.HandleFunc(constants.EndpointClustersPosture, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionRead,
			postureHandler,
		))
		mux.HandleFunc(constants.EndpointCloudResources, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			crHandler,
		))
		mux.HandleFunc(constants.EndpointCloudResourcesReconcile, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			reconcileHandler,
		))
		return
	}

	mux.HandleFunc(constants.EndpointClusters, clHandler)
	mux.HandleFunc(constants.EndpointClustersPosture, postureHandler)
	mux.HandleFunc(constants.EndpointCloudResources, crHandler)
	mux.HandleFunc(constants.EndpointCloudResourcesReconcile, reconcileHandler)
}
