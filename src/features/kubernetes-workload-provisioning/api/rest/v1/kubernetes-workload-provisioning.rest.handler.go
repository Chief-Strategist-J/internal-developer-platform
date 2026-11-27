/*
ALGORITHM & ARCHITECTURE BLUEPRINT: KUBERNETES WORKLOAD INGRESS HANDLER

1. Ingress Flow Execution:
  - Ingests W3C trace context, correlation, causation, and audit headers from HTTP transport.
  - Decodes closed-schema JSON payload with strict disallowance of unknown fields.
  - Enforces Tier-2 handler spans ("kubernetes_workload.handler.*") wrapping all endpoint executions.
  - Enriches command context with tenant ID and idempotency key from headers when omitted in payload.
  - Dispatches validated command to Domain Service.
  - Wraps result or typed application errors in RFC-compliant ApiResponse envelopes.

2. Error Mapping:
  - ApplicationError instances preserve HTTP status code, canonical code, and error messages.
  - Unhandled exceptions map to HTTP 500 INTERNAL_SERVER_ERROR.

3. Invariants:
  - Zero inline comments inside function bodies.
  - All functions strictly maintain a length <= 25 lines.
  - All headers and content types reference centralized package constants.
*/
package restv1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/service"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	appErrors "internal-developer-platform/src/shared/errors"
	"internal-developer-platform/src/shared/types"
)

type IngressMetadata struct {
	Traceparent    string
	RequestID      string
	CorrelationID  string
	CausationID    string
	TenantID       string
	ClientID       string
	UserID         string
	IdempotencyKey string
	APIVersion     string
	IfMatch        string
	StartTime      time.Time
}

type KubernetesWorkloadHandler struct {
	service *service.WorkloadProvisioningService
}

func NewKubernetesWorkloadHandler(s *service.WorkloadProvisioningService) *KubernetesWorkloadHandler {
	return &KubernetesWorkloadHandler{service: s}
}

func (h *KubernetesWorkloadHandler) ProvisionWorkload(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "kubernetes_workload.handler.provision", func(ctx context.Context) error {
		meta := extractIngressMetadata(r)
		cmd, err := decodeProvisionPayload(r, meta)
		if err != nil {
			detail := types.ResponseErrorDetail{
				Field: "body",
				Issue: err.Error(),
				Rule:  "closed_schema_json",
			}
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "malformed request payload", meta, detail)
			return err
		}
		result, err := h.service.ProvisionWorkload(ctx, cmd)
		if err != nil {
			h.handleServiceError(w, err, meta)
			return err
		}
		h.writeSuccessResponse(w, http.StatusCreated, result, meta)
		return nil
	})
}

func (h *KubernetesWorkloadHandler) GetWorkload(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "kubernetes_workload.handler.get", func(ctx context.Context) error {
		meta := extractIngressMetadata(r)
		id, err := extractWorkloadID(r.URL.Path)
		if err != nil {
			detail := types.ResponseErrorDetail{
				Field: "id",
				Issue: err.Error(),
				Rule:  "path_workload_id",
			}
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), meta, detail)
			return err
		}
		result, err := h.service.GetWorkload(ctx, meta.TenantID, id)
		if err != nil {
			h.handleServiceError(w, err, meta)
			return err
		}
		h.writeSuccessResponse(w, http.StatusOK, result, meta)
		return nil
	})
}

func (h *KubernetesWorkloadHandler) handleServiceError(w http.ResponseWriter, err error, meta IngressMetadata) {
	if appErr, ok := err.(*appErrors.ApplicationError); ok {
		h.writeErrorResponse(w, appErr.StatusCode, appErr.CanonicalCode, appErr.Message, meta)
		return
	}
	h.writeErrorResponse(w, http.StatusInternalServerError, constants.ErrCodeInternalError, "internal error", meta)
}

func extractIngressMetadata(r *http.Request) IngressMetadata {
	return IngressMetadata{
		Traceparent:    r.Header.Get(constants.HeaderTraceparent),
		RequestID:      r.Header.Get(constants.HeaderXRequestID),
		CorrelationID:  r.Header.Get(constants.HeaderXCorrelationID),
		CausationID:    r.Header.Get(constants.HeaderXCausationID),
		TenantID:       r.Header.Get(constants.HeaderXTenantID),
		ClientID:       r.Header.Get(constants.HeaderXClientID),
		UserID:         r.Header.Get(constants.HeaderXUserID),
		IdempotencyKey: r.Header.Get(constants.HeaderXIdempotencyKey),
		APIVersion:     r.Header.Get(constants.HeaderXAPIVersion),
		IfMatch:        r.Header.Get(constants.HeaderIfMatch),
		StartTime:      time.Now(),
	}
}

func decodeProvisionPayload(r *http.Request, meta IngressMetadata) (contracts.WorkloadProvisionCommand, error) {
	var cmd contracts.WorkloadProvisionCommand
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cmd); err != nil {
		return cmd, err
	}
	if cmd.TenantID == "" && meta.TenantID != "" {
		cmd.TenantID = meta.TenantID
	}
	if cmd.IdempotencyKey == "" && meta.IdempotencyKey != "" {
		cmd.IdempotencyKey = meta.IdempotencyKey
	}
	return cmd, nil
}

func extractWorkloadID(urlPath string) (string, error) {
	trimmed := strings.Trim(urlPath, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 4 {
		return "", errors.New("missing workload id in path")
	}
	id := parts[len(parts)-1]
	if id == "" {
		return "", errors.New("invalid workload id in path")
	}
	return id, nil
}

func (h *KubernetesWorkloadHandler) writeSuccessResponse(w http.ResponseWriter, status int, data any, meta IngressMetadata) {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	if meta.Traceparent != "" {
		w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)
	}
	if meta.RequestID != "" {
		w.Header().Set(constants.HeaderXRequestID, meta.RequestID)
	}
	w.WriteHeader(status)
	resp := types.ApiResponse{
		Success:    true,
		StatusCode: status,
		Data:       data,
		Meta:       buildMeta(meta),
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *KubernetesWorkloadHandler) writeErrorResponse(w http.ResponseWriter, status int, code, msg string, meta IngressMetadata, details ...types.ResponseErrorDetail) {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	if meta.Traceparent != "" {
		w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)
	}
	if meta.RequestID != "" {
		w.Header().Set(constants.HeaderXRequestID, meta.RequestID)
	}
	w.WriteHeader(status)
	errObj := &types.ResponseError{Code: code, Message: msg}
	if len(details) > 0 {
		errObj.Details = details
	}
	resp := types.ApiResponse{
		Success:    false,
		StatusCode: status,
		Error:      errObj,
		Meta:       buildMeta(meta),
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *KubernetesWorkloadHandler) RecordTimelineEvent(w http.ResponseWriter, r *http.Request) {
	meta := extractIngressMetadata(r)
	_ = observability.WithSpan(r.Context(), "kubernetes_workload.handler.record_timeline", func(ctx context.Context) error {
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		var ev contracts.IncidentTimelineEvent
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&ev); err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "malformed request payload", meta)
			return err
		}
		if ev.TenantID == "" {
			ev.TenantID = meta.TenantID
		}
		if err := h.service.RecordIncidentTimelineEvent(ctx, ev); err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), meta)
			return err
		}
		w.WriteHeader(http.StatusCreated)
		return json.NewEncoder(w).Encode(types.ApiResponse{
			Success:    true,
			StatusCode: http.StatusCreated,
			Data:       ev,
			Meta:       buildMeta(meta),
		})
	})
}

func (h *KubernetesWorkloadHandler) ListTimelineEvents(w http.ResponseWriter, r *http.Request) {
	meta := extractIngressMetadata(r)
	_ = observability.WithSpan(r.Context(), "kubernetes_workload.handler.list_timeline", func(ctx context.Context) error {
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		workloadID := r.URL.Query().Get("workloadId")
		events, err := h.service.ListIncidentTimelineEvents(ctx, meta.TenantID, workloadID, 50, 0)
		if err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), meta)
			return err
		}
		w.WriteHeader(http.StatusOK)
		return json.NewEncoder(w).Encode(types.ApiResponse{
			Success:    true,
			StatusCode: http.StatusOK,
			Data:       events,
			Meta:       buildMeta(meta),
		})
	})
}

func buildMeta(meta IngressMetadata) types.ResponseMeta {
	apiVer := meta.APIVersion
	if apiVer == "" {
		apiVer = "v1"
	}
	return types.ResponseMeta{
		RequestID:       meta.RequestID,
		CorrelationID:   meta.CorrelationID,
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		ExecutionTimeMs: time.Since(meta.StartTime).Milliseconds(),
		APIVersion:      apiVer,
	}
}
