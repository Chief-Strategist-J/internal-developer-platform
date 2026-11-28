/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DEPLOYMENT STRATEGIES REST HANDLER (V.3 role 02)

1. Purpose:
   - Exposes HTTP REST endpoints for progressive delivery (/api/v1/rollout-plans, /api/v1/rollouts).
   - Enforces trace propagation, tenant context resolution, and canonical JSON response envelopes.
2. Invariants:
   - Zero inline comments below header.
*/

package restv1

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"internal-developer-platform/src/features/deployment-strategies/internal/contracts"
	"internal-developer-platform/src/features/deployment-strategies/internal/service"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/kernel"
)

type DeploymentStrategiesHandler struct {
	svc *service.DeploymentStrategiesService
}

func NewDeploymentStrategiesHandler(svc *service.DeploymentStrategiesService) *DeploymentStrategiesHandler {
	return &DeploymentStrategiesHandler{svc: svc}
}

func (h *DeploymentStrategiesHandler) HandleCreatePlan(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "deployment_strategies.api.create_plan", func(ctx context.Context) error {
		meta := middleware.ExtractIngressMetadata(r)
		var plan contracts.RolloutPlan
		if err := json.NewDecoder(r.Body).Decode(&plan); err != nil {
			writeErrorResponse(w, http.StatusBadRequest, "INVALID_PAYLOAD", err.Error())
			return err
		}
		if plan.TenantID == "" {
			plan.TenantID = meta.TenantID
		}
		if plan.CorrelationID == "" {
			plan.CorrelationID = meta.CorrelationID
		}
		if err := h.svc.CreatePlan(ctx, plan); err != nil {
			writeErrorResponse(w, http.StatusUnprocessableEntity, "VALIDATION_FAILED", err.Error())
			return err
		}
		writeSuccessResponse(w, http.StatusCreated, plan)
		return nil
	})
}

func (h *DeploymentStrategiesHandler) HandleStartRollout(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "deployment_strategies.api.start_rollout", func(ctx context.Context) error {
		meta := middleware.ExtractIngressMetadata(r)
		var input contracts.StartRolloutInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeErrorResponse(w, http.StatusBadRequest, "INVALID_PAYLOAD", err.Error())
			return err
		}
		if input.TenantID == "" {
			input.TenantID = meta.TenantID
		}
		if input.CorrelationID == "" {
			input.CorrelationID = meta.CorrelationID
		}
		exec, err := h.svc.StartRollout(ctx, input)
		if err != nil {
			writeErrorResponse(w, http.StatusConflict, "ROLLOUT_START_FAILED", err.Error())
			return err
		}
		writeSuccessResponse(w, http.StatusAccepted, exec)
		return nil
	})
}

func (h *DeploymentStrategiesHandler) HandleGetRollout(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "deployment_strategies.api.get_rollout", func(ctx context.Context) error {
		meta := middleware.ExtractIngressMetadata(r)
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 3 {
			writeErrorResponse(w, http.StatusBadRequest, "INVALID_PATH", "missing executionId")
			return nil
		}
		executionID := parts[len(parts)-1]
		exec, err := h.svc.GetExecution(ctx, meta.TenantID, executionID)
		if err != nil {
			writeErrorResponse(w, http.StatusNotFound, "NOT_FOUND", err.Error())
			return err
		}
		writeSuccessResponse(w, http.StatusOK, exec)
		return nil
	})
}

func writeSuccessResponse[T any](w http.ResponseWriter, status int, data T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	env := kernel.Envelope[T]{
		Success:    true,
		StatusCode: status,
		Data:       &data,
	}
	_ = json.NewEncoder(w).Encode(env)
}

func writeErrorResponse(w http.ResponseWriter, status int, title, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	env := kernel.Envelope[any]{
		Success:    false,
		StatusCode: status,
		Error: &kernel.Error{
			Type:   "about:blank",
			Title:  title,
			Status: status,
			Detail: detail,
		},
	}
	_ = json.NewEncoder(w).Encode(env)
}
