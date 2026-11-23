package routers

import (
	"net/http"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/middleware"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/shared/constants"
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

	if r.rbacMiddle != nil {
		mux.HandleFunc(constants.EndpointClusters, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceInfrastructure,
			rbacContracts.ActionWrite,
			clHandler,
		))
		mux.HandleFunc(constants.EndpointCloudResources, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceInfrastructure,
			rbacContracts.ActionWrite,
			crHandler,
		))
		return
	}

	mux.HandleFunc(constants.EndpointClusters, clHandler)
	mux.HandleFunc(constants.EndpointCloudResources, crHandler)
}
