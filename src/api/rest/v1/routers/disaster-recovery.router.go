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
      * Restores: POST /api/v1/dr/restores
      * Breakglass: POST /api/v1/dr/breakglass:activate
  - Attaches RBAC permission gates when middleware is provided.

2. Invariants:
  - Zero inline comments inside function bodies.
  - All paths reference system constants.
*/
package routers

import (
	"net/http"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/kernel/capabilities/authz"
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
	evidenceHandler := http.HandlerFunc(r.handler.HandleEvidence)
	restoresHandler := http.HandlerFunc(r.handler.HandleRestores)
	breakglassHandler := http.HandlerFunc(r.handler.HandleBreakglass)

	if r.rbacMiddle != nil {
		mux.HandleFunc(constants.EndpointDRPlans, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			plansHandler,
		))
		mux.HandleFunc(constants.EndpointDRPlansPrefix, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionRead,
			singlePlanHandler,
		))
		mux.HandleFunc(constants.EndpointDRIncidentsDeclare, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			declareIncidentHandler,
		))
		mux.HandleFunc(constants.EndpointDRIncidentsApprovePrefix, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			approveIncidentHandler,
		))
		mux.HandleFunc(constants.EndpointDRFailovers, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			failoverHandler,
		))
		mux.HandleFunc(constants.EndpointDRFailoversPrefix, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			singleFailoverHandler,
		))
		mux.HandleFunc(constants.EndpointDRFailbacks, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			failbackHandler,
		))
		mux.HandleFunc(constants.EndpointDRFailbacksPrefix, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionRead,
			failbackHandler,
		))
		mux.HandleFunc(constants.EndpointDRDatasetsPrefix, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			datasetsHandler,
		))
		mux.HandleFunc(constants.EndpointDRDrills, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			drillsHandler,
		))
		mux.HandleFunc(constants.EndpointDRDrillsPrefix, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionRead,
			drillsHandler,
		))
		mux.HandleFunc(constants.EndpointDREvidencePrefix, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionRead,
			evidenceHandler,
		))
		mux.HandleFunc(constants.EndpointDRRestores, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			restoresHandler,
		))
		mux.HandleFunc(constants.EndpointDRRestoresPrefix, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionRead,
			restoresHandler,
		))
		mux.HandleFunc(constants.EndpointDRBreakglassActivate, r.rbacMiddle.RequirePermission(
			authz.ResourceInfrastructure,
			authz.ActionWrite,
			breakglassHandler,
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
	mux.HandleFunc(constants.EndpointDRFailbacksPrefix, failbackHandler)
	mux.HandleFunc(constants.EndpointDRDatasetsPrefix, datasetsHandler)
	mux.HandleFunc(constants.EndpointDRDrills, drillsHandler)
	mux.HandleFunc(constants.EndpointDRDrillsPrefix, drillsHandler)
	mux.HandleFunc(constants.EndpointDREvidencePrefix, evidenceHandler)
	mux.HandleFunc(constants.EndpointDRRestores, restoresHandler)
	mux.HandleFunc(constants.EndpointDRRestoresPrefix, restoresHandler)
	mux.HandleFunc(constants.EndpointDRBreakglassActivate, breakglassHandler)
}
