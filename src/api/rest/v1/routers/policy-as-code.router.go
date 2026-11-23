/*
ALGORITHM & ARCHITECTURE BLUEPRINT: POLICY-AS-CODE REST INGRESS ROUTER

1. Routing Flow Execution:
   - Registers declarative endpoints for Policy-as-Code API.
   - Binds root collection endpoint /api/v1/policies (POST create, GET list).
   - Binds admission evaluation endpoint /api/v1/policies/admission/evaluate (POST).
   - Binds parameterized subresource routes /api/v1/policies/{id}, /lifecycle, and /exceptions.
   - Attaches RBAC middleware permission checks when provided.
   - Zero inline comments inside function bodies.
*/
package routers

import (
	"net/http"
	"strings"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/middleware"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/shared/constants"
)

type PolicyAsCodeRouter struct {
	handler    *handlers.PolicyAsCodeHandler
	rbacMiddle *middleware.RBACMiddleware
}

func NewPolicyAsCodeRouter(h *handlers.PolicyAsCodeHandler, m *middleware.RBACMiddleware) *PolicyAsCodeRouter {
	return &PolicyAsCodeRouter{
		handler:    h,
		rbacMiddle: m,
	}
}

func (r *PolicyAsCodeRouter) RegisterRoutes(mux *http.ServeMux) {
	collectionHandler := r.buildCollectionHandler()
	subresourceHandler := r.buildSubresourceHandler()
	admissionHandler := r.buildAdmissionHandler()

	if r.rbacMiddle != nil {
		mux.HandleFunc(constants.EndpointPoliciesEvaluate, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceInfrastructure,
			rbacContracts.ActionRead,
			admissionHandler,
		))
		mux.HandleFunc(constants.EndpointPolicies, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceInfrastructure,
			rbacContracts.ActionWrite,
			collectionHandler,
		))
		mux.HandleFunc(constants.EndpointPoliciesPrefix, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceInfrastructure,
			rbacContracts.ActionWrite,
			subresourceHandler,
		))
		return
	}

	mux.HandleFunc(constants.EndpointPoliciesEvaluate, admissionHandler)
	mux.HandleFunc(constants.EndpointPolicies, collectionHandler)
	mux.HandleFunc(constants.EndpointPoliciesPrefix, subresourceHandler)
}

func (r *PolicyAsCodeRouter) buildCollectionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			r.handler.CreatePolicy(w, req)
		case http.MethodGet:
			r.handler.ListActivePolicies(w, req)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}
}

func (r *PolicyAsCodeRouter) buildSubresourceHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		path := req.URL.Path
		if strings.HasSuffix(path, "/lifecycle") && (req.Method == http.MethodPost) {
			r.handler.TransitionLifecycle(w, req)
			return
		}
		if strings.HasSuffix(path, "/exceptions") && (req.Method == http.MethodPost) {
			r.handler.RequestException(w, req)
			return
		}
		if req.Method == http.MethodGet {
			r.handler.GetPolicy(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (r *PolicyAsCodeRouter) buildAdmissionHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.handler.EvaluateAdmission(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
