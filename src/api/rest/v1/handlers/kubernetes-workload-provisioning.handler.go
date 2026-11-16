package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	kubernetesworkloadprovisioning "internal-developer-platform/src/features/kubernetes-workload-provisioning"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/shared/constants"
	appErrors "internal-developer-platform/src/shared/errors"
	"internal-developer-platform/src/shared/types"
)

type KubernetesWorkloadHandler struct {
	service *kubernetesworkloadprovisioning.Service
}

func NewKubernetesWorkloadHandler(s *kubernetesworkloadprovisioning.Service) *KubernetesWorkloadHandler {
	return &KubernetesWorkloadHandler{service: s}
}

func (h *KubernetesWorkloadHandler) ProvisionWorkload(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	w.Header().Set("Content-Type", "application/json")
	traceparent := r.Header.Get(constants.HeaderTraceparent)
	reqID := r.Header.Get(constants.HeaderXRequestID)
	corrID := r.Header.Get(constants.HeaderXCorrelationID)
	tenantID := r.Header.Get(constants.HeaderXTenantID)
	idempotencyKey := r.Header.Get(constants.HeaderXIdempotencyKey)

	var cmd contracts.WorkloadProvisionCommand
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cmd); err != nil {
		h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "malformed request payload", start, reqID, corrID)
		return
	}

	if cmd.TenantID == "" && tenantID != "" {
		cmd.TenantID = tenantID
	}
	if cmd.IdempotencyKey == "" && idempotencyKey != "" {
		cmd.IdempotencyKey = idempotencyKey
	}

	result, err := h.service.ProvisionWorkload(r.Context(), cmd)
	if err != nil {
		if appErr, ok := err.(*appErrors.ApplicationError); ok {
			h.writeError(w, appErr.StatusCode, appErr.CanonicalCode, appErr.Message, start, reqID, corrID)
			return
		}
		h.writeError(w, http.StatusInternalServerError, constants.ErrCodeInternalError, "internal error", start, reqID, corrID)
		return
	}

	w.WriteHeader(http.StatusCreated)
	if traceparent != "" {
		w.Header().Set(constants.HeaderTraceparent, traceparent)
	}

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
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *KubernetesWorkloadHandler) GetWorkload(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	w.Header().Set("Content-Type", "application/json")
	traceparent := r.Header.Get(constants.HeaderTraceparent)
	reqID := r.Header.Get(constants.HeaderXRequestID)
	corrID := r.Header.Get(constants.HeaderXCorrelationID)
	tenantID := r.Header.Get(constants.HeaderXTenantID)

	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "missing workload id in path", start, reqID, corrID)
		return
	}
	id := pathParts[len(pathParts)-1]

	result, err := h.service.GetWorkload(r.Context(), tenantID, id)
	if err != nil {
		if appErr, ok := err.(*appErrors.ApplicationError); ok {
			h.writeError(w, appErr.StatusCode, appErr.CanonicalCode, appErr.Message, start, reqID, corrID)
			return
		}
		h.writeError(w, http.StatusInternalServerError, constants.ErrCodeInternalError, "internal error", start, reqID, corrID)
		return
	}

	w.WriteHeader(http.StatusOK)
	if traceparent != "" {
		w.Header().Set(constants.HeaderTraceparent, traceparent)
	}

	resp := types.ApiResponse{
		Success:    true,
		StatusCode: http.StatusOK,
		Data:       result,
		Meta: types.ResponseMeta{
			RequestID:       reqID,
			CorrelationID:   corrID,
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
			ExecutionTimeMs: time.Since(start).Milliseconds(),
			APIVersion:      "v1",
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *KubernetesWorkloadHandler) writeError(w http.ResponseWriter, status int, code string, msg string, start time.Time, reqID string, corrID string) {
	w.WriteHeader(status)
	resp := types.ApiResponse{
		Success:    false,
		StatusCode: status,
		Error: &types.ResponseError{
			Code:    code,
			Message: msg,
		},
		Meta: types.ResponseMeta{
			RequestID:       reqID,
			CorrelationID:   corrID,
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
			ExecutionTimeMs: time.Since(start).Milliseconds(),
			APIVersion:      "v1",
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

