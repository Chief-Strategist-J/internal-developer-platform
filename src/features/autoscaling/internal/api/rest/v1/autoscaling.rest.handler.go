/*
ALGORITHM & ARCHITECTURE BLUEPRINT: AUTOSCALING REST HANDLER (V.3 role 02, API Guardrails 1-4, 6)

1. Purpose:
   - POST /api/v1/autoscaling/plans:evaluate ingests transport headers, parses a closed-schema body,
     takes the tenant from middleware-verified headers only, calls the service port inside a span,
     and answers with the canonical envelope (success, statusCode, data|error, meta).
2. Invariants:
   - Unknown JSON fields are rejected (additionalProperties: false).
   - The tenant never comes from the body (P1-2); traceparent, x-request-id and x-correlation-id are echoed.
   - A rule violation is a 200 with allowed=false; only malformed input is a 4xx.
   - Zero inline comments below this header.
*/

package restv1

import (
	"context"
	"encoding/json"
	"net/http"

	"internal-developer-platform/src/features/autoscaling/internal/contracts"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/types"
	"internal-developer-platform/src/shared/utils"
)

type AutoscalingHandler struct {
	svc contracts.AutoscalingServicePort
}

func NewAutoscalingHandler(svc contracts.AutoscalingServicePort) *AutoscalingHandler {
	return &AutoscalingHandler{svc: svc}
}

func (h *AutoscalingHandler) HandleEvaluatePlan(w http.ResponseWriter, r *http.Request) {
	meta := middleware.ExtractIngressMetadata(r)
	_ = observability.WithSpan(r.Context(), "autoscaling.api.evaluate_plan", func(ctx context.Context) error {
		input, err := mapRequestToInput(r, meta)
		if err != nil {
			writeErrorResponse(w, meta, http.StatusBadRequest, constants.ErrCodeInvalidPayload, err.Error())
			return err
		}
		evaluation, err := h.svc.EvaluatePlan(ctx, input)
		if err != nil {
			writeErrorResponse(w, meta, http.StatusUnprocessableEntity, constants.ErrCodeValidationFailed, err.Error())
			return err
		}
		writeSuccessResponse(w, meta, http.StatusOK, evaluation)
		return nil
	})
}

func mapRequestToInput(r *http.Request, meta utils.IngressMetadata) (contracts.EvaluatePlanInput, error) {
	var input contracts.EvaluatePlanInput
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return contracts.EvaluatePlanInput{}, err
	}
	input.Plan.TenantID = meta.TenantID
	return input, nil
}

func writeEchoHeaders(w http.ResponseWriter, meta utils.IngressMetadata) {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)
	w.Header().Set(constants.HeaderXRequestID, meta.RequestID)
	w.Header().Set(constants.HeaderXCorrelationID, meta.CorrelationID)
}

func writeSuccessResponse(w http.ResponseWriter, meta utils.IngressMetadata, status int, data contracts.Evaluation) {
	writeEchoHeaders(w, meta)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(types.ApiResponse{
		Success:    true,
		StatusCode: status,
		Data:       data,
		Meta:       utils.BuildResponseMeta(meta),
	})
}

func writeErrorResponse(w http.ResponseWriter, meta utils.IngressMetadata, status int, code, message string) {
	writeEchoHeaders(w, meta)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(types.ApiResponse{
		Success:    false,
		StatusCode: status,
		Error:      &types.ResponseError{Code: code, Message: message, Retryable: false},
		Meta:       utils.BuildResponseMeta(meta),
	})
}
