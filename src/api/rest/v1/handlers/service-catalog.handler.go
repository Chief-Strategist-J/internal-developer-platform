// Package handlers provides HTTP REST handlers for Service Catalog.
//
// Architectural Blueprint:
// - Step 02 Handler: Ingress delivery boundary for Service Catalog.
// - Spawns Tier-2 handler child spans via observability.WithSpan.
// - 100% comment-free function bodies.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"

	catalogContracts "internal-developer-platform/src/features/service-catalog/contracts"
	catalogService "internal-developer-platform/src/features/service-catalog/service"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	appErrors "internal-developer-platform/src/shared/errors"
	"internal-developer-platform/src/shared/types"
)

type ServiceCatalogHandler struct {
	service *catalogService.ServiceCatalogService
}

func NewServiceCatalogHandler(s *catalogService.ServiceCatalogService) *ServiceCatalogHandler {
	return &ServiceCatalogHandler{service: s}
}

func (h *ServiceCatalogHandler) RegisterService(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "service_catalog.handler.register", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)

		var input catalogContracts.RegisterServiceInput
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "malformed request payload", start, reqID, corrID)
			return err
		}

		result, err := h.service.RegisterService(ctx, input)
		if err != nil {
			if appErr, ok := err.(*appErrors.ApplicationError); ok {
				h.writeError(w, appErr.StatusCode, appErr.CanonicalCode, appErr.Message, start, reqID, corrID)
				return err
			}
			h.writeError(w, http.StatusInternalServerError, constants.ErrCodeInternalError, "internal error", start, reqID, corrID)
			return err
		}

		w.WriteHeader(http.StatusCreated)
		w.Header().Set(constants.HeaderTraceparent, traceparent)
		resp := types.ApiResponse{
			Success:    true,
			StatusCode: http.StatusCreated,
			Data:       result,
			Meta: types.ResponseMeta{
				RequestID:       reqID,
				CorrelationID:   corrID,
				Timestamp:       time.Now().UTC().Format(time.RFC3339),
				ExecutionTimeMs: time.Since(start).Milliseconds(),
				APIVersion:      "v1",
			},
		}
		json.NewEncoder(w).Encode(resp)
		return nil
	}, attribute.String("http.method", r.Method))
}

func (h *ServiceCatalogHandler) writeError(w http.ResponseWriter, code int, errCode string, msg string, start time.Time, reqID string, corrID string) {
	w.WriteHeader(code)
	resp := types.ApiResponse{
		Success:    false,
		StatusCode: code,
		Error: &types.ResponseError{
			Code:      errCode,
			Message:   msg,
			Retryable: code == http.StatusTooManyRequests || code == http.StatusServiceUnavailable,
		},
		Meta: types.ResponseMeta{
			RequestID:       reqID,
			CorrelationID:   corrID,
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
			ExecutionTimeMs: time.Since(start).Milliseconds(),
			APIVersion:      "v1",
		},
	}
	json.NewEncoder(w).Encode(resp)
}
