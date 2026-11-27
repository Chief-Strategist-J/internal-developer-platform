/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PLATFORM REGIME REST HANDLER

1. Ingress Flow Execution:
  - Ingests W3C traceparent, correlation, causation, and tenant headers.
  - Decodes closed-schema JSON payload for POST /api/v1/regimes.
  - Enforces Tier-2 handler spans ("platform_regime.handler.*") wrapping all endpoint executions.
  - Dispatches to PlatformRegimeService under child span.
  - Returns typed ApiResponse envelopes with no comments inside function bodies.
*/
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	platformregime "internal-developer-platform/src/features/platform-regime"
	"internal-developer-platform/src/features/platform-regime/contracts"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	appErrors "internal-developer-platform/src/shared/errors"
	"internal-developer-platform/src/shared/utils"
)

type PlatformRegimeHandler struct {
	service *platformregime.PlatformRegimeService
}

func NewPlatformRegimeHandler(s *platformregime.PlatformRegimeService) *PlatformRegimeHandler {
	return &PlatformRegimeHandler{service: s}
}

func (h *PlatformRegimeHandler) RegisterRegimePolicy(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "platform_regime.handler.register_policy", func(ctx context.Context) error {
		meta := utils.ExtractIngressMetadata(r)
		cmd, err := decodeRegimePayload(r, meta)
		if err != nil {
			h.handleError(w, meta, err)
			return err
		}

		record, err := h.service.RegisterRegimePolicy(ctx, cmd)
		if err != nil {
			h.handleError(w, meta, err)
			return err
		}

		h.writeSuccessResponse(w, http.StatusCreated, record, meta)
		return nil
	})
}

func (h *PlatformRegimeHandler) GetActiveRegimePolicy(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "platform_regime.handler.get_active_policy", func(ctx context.Context) error {
		meta := utils.ExtractIngressMetadata(r)
		tenantID := r.URL.Query().Get("tenantId")
		if tenantID == "" {
			tenantID = meta.TenantID
		}
		if tenantID == "" {
			err := appErrors.NewValidationError("tenantId query parameter or header is required")
			h.handleError(w, meta, err)
			return err
		}

		record, err := h.service.GetActiveRegimePolicy(ctx, tenantID)
		if err != nil {
			h.handleError(w, meta, err)
			return err
		}

		h.writeSuccessResponse(w, http.StatusOK, record, meta)
		return nil
	})
}

func (h *PlatformRegimeHandler) handleError(w http.ResponseWriter, meta utils.IngressMetadata, err error) {
	var appErr *appErrors.ApplicationError
	if errors.As(err, &appErr) {
		h.writeErrorResponse(w, appErr.StatusCode, appErr.CanonicalCode, appErr.Message, meta)
		return
	}
	h.writeErrorResponse(w, http.StatusInternalServerError, constants.ErrCodeInternalError, "internal server error", meta)
}

func (h *PlatformRegimeHandler) writeSuccessResponse(w http.ResponseWriter, status int, data any, meta utils.IngressMetadata) {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	if meta.Traceparent != "" {
		w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)
	}
	if meta.RequestID != "" {
		w.Header().Set(constants.HeaderXRequestID, meta.RequestID)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"statusCode": status,
		"data":       data,
		"meta":       utils.BuildResponseMeta(meta),
	})
}

func (h *PlatformRegimeHandler) writeErrorResponse(w http.ResponseWriter, status int, code, msg string, meta utils.IngressMetadata) {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	if meta.Traceparent != "" {
		w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)
	}
	if meta.RequestID != "" {
		w.Header().Set(constants.HeaderXRequestID, meta.RequestID)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":    false,
		"statusCode": status,
		"error": map[string]any{
			"code":    code,
			"message": msg,
		},
		"meta": utils.BuildResponseMeta(meta),
	})
}

func decodeRegimePayload(r *http.Request, meta utils.IngressMetadata) (contracts.CreateRegimePolicyCommand, error) {
	var cmd contracts.CreateRegimePolicyCommand
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cmd); err != nil {
		return cmd, appErrors.NewValidationError("invalid request payload: " + err.Error())
	}
	if cmd.TenantID == "" {
		cmd.TenantID = meta.TenantID
	}
	if cmd.CorrelationID == "" {
		cmd.CorrelationID = meta.CorrelationID
	}
	if err := contracts.ValidateCreateRegimePolicyCommand(cmd); err != nil {
		return cmd, err
	}
	return cmd, nil
}
