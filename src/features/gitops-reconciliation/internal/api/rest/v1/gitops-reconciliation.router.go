package restv1

import (
	"net/http"
	"strings"

	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/kernel/capabilities/authz"
)

type GitOpsReconciliationRouter struct {
	handler    *GitOpsReconciliationHandler
	rbacMiddle *middleware.RBACMiddleware
}

func NewGitOpsReconciliationRouter(h *GitOpsReconciliationHandler, m *middleware.RBACMiddleware) *GitOpsReconciliationRouter {
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
		case http.MethodGet:
			r.handler.ListReconciliations(w, req)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	singleReconcileHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasSuffix(req.URL.Path, "/diff") && req.Method == http.MethodGet {
			r.handler.GetDriftDiff(w, req)
			return
		}
		if strings.HasPrefix(req.URL.Path, constants.EndpointReconciliationsPrefix) && req.Method == http.MethodGet {
			r.handler.GetReconciliation(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	webhookHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.handler.HandleWebhook(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	secretConfigHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodPut, http.MethodPost:
			r.handler.ConfigureSecretMechanism(w, req)
		case http.MethodGet:
			r.handler.GetSecretMechanismConfig(w, req)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	secretValidateHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.handler.ValidateSecretManifest(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	fleetRolloutHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodPost {
			r.handler.ExecuteFleetRollout(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	mux.HandleFunc(constants.EndpointGitOpsSecretsConfig, secretConfigHandler)
	mux.HandleFunc(constants.EndpointGitOpsSecretsValidate, secretValidateHandler)
	mux.HandleFunc(constants.EndpointGitOpsFleetRollout, fleetRolloutHandler)

	if r.rbacMiddle != nil {
		mux.HandleFunc(constants.EndpointReconciliations, r.rbacMiddle.RequirePermission(
			authz.ResourceServiceCatalog,
			authz.ActionWrite,
			reconcileHandler,
		))
		mux.HandleFunc(constants.EndpointReconciliationsPrefix, r.rbacMiddle.RequirePermission(
			authz.ResourceServiceCatalog,
			authz.ActionRead,
			singleReconcileHandler,
		))
	} else {
		mux.HandleFunc(constants.EndpointReconciliations, reconcileHandler)
		mux.HandleFunc(constants.EndpointReconciliationsPrefix, singleReconcileHandler)
	}

	mux.HandleFunc(constants.EndpointGitOpsWebhook, webhookHandler)
}
