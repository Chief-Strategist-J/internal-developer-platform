package routers

import (
	"net/http"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/kernel/capabilities/authz"
)

type OfferingsRouter struct {
	clusterHandler       *handlers.ClusterHandler
	cloudResourceHandler *handlers.CloudResourceHandler
	rbacMiddle           *middleware.RBACMiddleware
}

func NewOfferingsRouter(clh *handlers.ClusterHandler, crh *handlers.CloudResourceHandler, rbacM *middleware.RBACMiddleware) *OfferingsRouter {
	return &OfferingsRouter{
		clusterHandler:       clh,
		cloudResourceHandler: crh,
		rbacMiddle:           rbacM,
	}
}

func (r *OfferingsRouter) RegisterRoutes(mux *http.ServeMux) {
	clHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			r.clusterHandler.ProvisionCluster(w, req)
		case http.MethodGet:
			r.clusterHandler.GetCluster(w, req)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	crHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.cloudResourceHandler.ProvisionResource(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	postureHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet {
			r.clusterHandler.ValidatePosture(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	reconcileHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.cloudResourceHandler.ReconcileLedger(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

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
