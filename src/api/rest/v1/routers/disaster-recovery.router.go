/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DISASTER RECOVERY INGRESS ROUTER

1. Routing Flow Execution:
  - Registers declarative endpoints for Disaster Recovery Control API:
      * Plans: POST/GET /api/v1/dr/plans, GET /api/v1/dr/plans/{plan}, POST /api/v1/dr/plans/{plan}:checkReadiness
      * Incidents: POST /api/v1/dr/incidents:declare, POST /api/v1/dr/incidents/{incident}:approve
      * Failovers: POST /api/v1/dr/failovers, POST /api/v1/dr/failovers/{op}:approve, POST /api/v1/dr/failovers/{op}:abort
      * Failbacks: POST /api/v1/dr/failbacks
      * Datasets & Fencing: POST /api/v1/dr/datasets/{dataset}:fence, GET /api/v1/dr/datasets/{dataset}/epoch
      * Drills: POST /api/v1/dr/drills
  - Attaches RBAC permission gates when middleware is provided.

2. Invariants:
  - Zero inline comments inside function bodies.
  - All paths reference system constants.
*/
package routers

import (
	"net/http"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/middleware"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/shared/constants"
)

type DisasterRecoveryRouter struct {
	handler    *handlers.DisasterRecoveryHandler
	rbacMiddle *middleware.RBACMiddleware
}

func NewDisasterRecoveryRouter(h *handlers.DisasterRecoveryHandler, m *middleware.RBACMiddleware) *DisasterRecoveryRouter {
	return &DisasterRecoveryRouter{
		handler:    h,
		rbacMiddle: m,
	}
}

func (r *DisasterRecoveryRouter) RegisterRoutes(mux *http.ServeMux) {
	plansHandler := http.HandlerFunc(r.handler.HandlePlans)
	singlePlanHandler := http.HandlerFunc(r.handler.HandleSinglePlan)
	declareIncidentHandler := http.HandlerFunc(r.handler.HandleDeclareIncident)
	approveIncidentHandler := http.HandlerFunc(r.handler.HandleApproveIncident)
	failoverHandler := http.HandlerFunc(r.handler.HandleFailovers)
	singleFailoverHandler := http.HandlerFunc(r.handler.HandleSingleFailover)
	failbackHandler := http.HandlerFunc(r.handler.HandleFailback)
	datasetsHandler := http.HandlerFunc(r.handler.HandleDatasets)
	drillsHandler := http.HandlerFunc(r.handler.HandleDrills)

	if r.rbacMiddle != nil {
		mux.HandleFunc(constants.EndpointDRPlans, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceInfrastructure,
			rbacContracts.ActionWrite,
			plansHandler,
		))
		mux.HandleFunc(constants.EndpointDRPlansPrefix, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceInfrastructure,
			rbacContracts.ActionRead,
			singlePlanHandler,
		))
		mux.HandleFunc(constants.EndpointDRIncidentsDeclare, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceInfrastructure,
			rbacContracts.ActionWrite,
			declareIncidentHandler,
		))
		mux.HandleFunc(constants.EndpointDRIncidentsApprovePrefix, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceInfrastructure,
			rbacContracts.ActionWrite,
			approveIncidentHandler,
		))
		mux.HandleFunc(constants.EndpointDRFailovers, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceInfrastructure,
			rbacContracts.ActionWrite,
			failoverHandler,
		))
		mux.HandleFunc(constants.EndpointDRFailoversPrefix, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceInfrastructure,
			rbacContracts.ActionWrite,
			singleFailoverHandler,
		))
		mux.HandleFunc(constants.EndpointDRFailbacks, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceInfrastructure,
			rbacContracts.ActionWrite,
			failbackHandler,
		))
		mux.HandleFunc(constants.EndpointDRDatasetsPrefix, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceInfrastructure,
			rbacContracts.ActionWrite,
			datasetsHandler,
		))
		mux.HandleFunc(constants.EndpointDRDrills, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceInfrastructure,
			rbacContracts.ActionWrite,
			drillsHandler,
		))
		return
	}

	mux.HandleFunc(constants.EndpointDRPlans, plansHandler)
	mux.HandleFunc(constants.EndpointDRPlansPrefix, singlePlanHandler)
	mux.HandleFunc(constants.EndpointDRIncidentsDeclare, declareIncidentHandler)
	mux.HandleFunc(constants.EndpointDRIncidentsApprovePrefix, approveIncidentHandler)
	mux.HandleFunc(constants.EndpointDRFailovers, failoverHandler)
	mux.HandleFunc(constants.EndpointDRFailoversPrefix, singleFailoverHandler)
	mux.HandleFunc(constants.EndpointDRFailbacks, failbackHandler)
	mux.HandleFunc(constants.EndpointDRDatasetsPrefix, datasetsHandler)
	mux.HandleFunc(constants.EndpointDRDrills, drillsHandler)
}
