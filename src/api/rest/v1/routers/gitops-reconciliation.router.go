package routers

import (
	"net/http"
	"strings"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/middleware"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/shared/constants"
)

type GitOpsReconciliationRouter struct {
	handler    *handlers.GitOpsReconciliationHandler
	rbacMiddle *middleware.RBACMiddleware
}

func NewGitOpsReconciliationRouter(h *handlers.GitOpsReconciliationHandler, m *middleware.RBACMiddleware) *GitOpsReconciliationRouter {
	return &GitOpsReconciliationRouter{
		handler:    h,
		rbacMiddle: m,
	}
}

func (r *GitOpsReconciliationRouter) RegisterRoutes(mux *http.ServeMux) {
	reconcileHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPost:
			r.handler.TriggerSync(w, req)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	singleReconcileHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, constants.EndpointReconciliationsPrefix) && req.Method == http.MethodGet {
			r.handler.GetReconciliation(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	if r.rbacMiddle != nil {
		mux.HandleFunc(constants.EndpointReconciliations, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceServiceCatalog,
			rbacContracts.ActionWrite,
			reconcileHandler,
		))
		mux.HandleFunc(constants.EndpointReconciliationsPrefix, r.rbacMiddle.RequirePermission(
			rbacContracts.ResourceServiceCatalog,
			rbacContracts.ActionRead,
			singleReconcileHandler,
		))
	} else {
		mux.HandleFunc(constants.EndpointReconciliations, reconcileHandler)
		mux.HandleFunc(constants.EndpointReconciliationsPrefix, singleReconcileHandler)
	}
}
