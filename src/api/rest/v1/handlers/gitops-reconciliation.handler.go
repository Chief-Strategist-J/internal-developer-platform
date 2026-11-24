package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	gitopsService "internal-developer-platform/src/features/gitops-reconciliation/service"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/types"
	"internal-developer-platform/src/shared/utils"
)

type GitOpsReconciliationHandler struct {
	service       *gitopsService.GitOpsReconciliationService
	webhookSecret string
}

func NewGitOpsReconciliationHandler(s *gitopsService.GitOpsReconciliationService) *GitOpsReconciliationHandler {
	secret := os.Getenv("GITOPS_WEBHOOK_SECRET")
	return &GitOpsReconciliationHandler{
		service:       s,
		webhookSecret: secret,
	}
}

func (h *GitOpsReconciliationHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	meta := utils.ExtractIngressMetadata(r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)

	var cmd contracts.SyncCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid json request body", start, meta.RequestID, meta.CorrelationID)
		return
	}

	if cmd.TenantID == "" {
		cmd.TenantID = meta.TenantID
	}
	if cmd.IdempotencyKey == "" {
		cmd.IdempotencyKey = r.Header.Get(constants.HeaderXIdempotencyKey)
	}

	result, err := h.service.TriggerReconciliation(r.Context(), cmd)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if strings.Contains(err.Error(), "validation") {
			status = http.StatusBadRequest
			code = "VALIDATION_FAILED"
		} else if strings.Contains(err.Error(), "conflict") || strings.Contains(err.Error(), "transition") {
			status = http.StatusConflict
			code = "CONFLICT"
		} else if strings.Contains(err.Error(), "prohibited") || strings.Contains(err.Error(), "forbidden") {
			status = http.StatusForbidden
			code = "FORBIDDEN"
		}
		h.writeError(w, status, code, err.Error(), start, meta.RequestID, meta.CorrelationID)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	resp := types.ApiResponse{
		Success:    true,
		StatusCode: http.StatusAccepted,
		Data:       result,
		Meta: types.ResponseMeta{
			RequestID:       meta.RequestID,
			CorrelationID:   meta.CorrelationID,
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
			ExecutionTimeMs: time.Since(start).Milliseconds(),
			APIVersion:      "v1",
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *GitOpsReconciliationHandler) GetReconciliation(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	meta := utils.ExtractIngressMetadata(r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)

	path := strings.TrimPrefix(r.URL.Path, constants.EndpointReconciliationsPrefix)
	id := strings.TrimSuffix(path, "/diff")

	record, err := h.service.GetReconciliation(r.Context(), meta.TenantID, id)
	if err != nil || record == nil {
		h.writeError(w, http.StatusNotFound, "NOT_FOUND", "reconciliation not found", start, meta.RequestID, meta.CorrelationID)
		return
	}

	w.WriteHeader(http.StatusOK)
	resp := types.ApiResponse{
		Success:    true,
		StatusCode: http.StatusOK,
		Data:       record,
		Meta: types.ResponseMeta{
			RequestID:       meta.RequestID,
			CorrelationID:   meta.CorrelationID,
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
			ExecutionTimeMs: time.Since(start).Milliseconds(),
			APIVersion:      "v1",
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *GitOpsReconciliationHandler) GetDriftDiff(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	meta := utils.ExtractIngressMetadata(r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)

	path := strings.TrimPrefix(r.URL.Path, constants.EndpointReconciliationsPrefix)
	id := strings.TrimSuffix(path, "/diff")

	diffReport, err := h.service.InspectDrift(r.Context(), meta.TenantID, id)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		h.writeError(w, status, code, err.Error(), start, meta.RequestID, meta.CorrelationID)
		return
	}

	w.WriteHeader(http.StatusOK)
	resp := types.ApiResponse{
		Success:    true,
		StatusCode: http.StatusOK,
		Data:       diffReport,
		Meta: types.ResponseMeta{
			RequestID:       meta.RequestID,
			CorrelationID:   meta.CorrelationID,
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
			ExecutionTimeMs: time.Since(start).Milliseconds(),
			APIVersion:      "v1",
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *GitOpsReconciliationHandler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	meta := utils.ExtractIngressMetadata(r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "failed to read webhook payload", start, meta.RequestID, meta.CorrelationID)
		return
	}

	var payload contracts.GitOpsWebhookPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		h.writeError(w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid json payload", start, meta.RequestID, meta.CorrelationID)
		return
	}

	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		signature = r.Header.Get("X-ArgoCD-Signature")
	}

	result, err := h.service.IngestWebhook(r.Context(), payload, signature, h.webhookSecret, bodyBytes)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if strings.Contains(err.Error(), "signature") || strings.Contains(err.Error(), "unauthorized") {
			status = http.StatusUnauthorized
			code = "UNAUTHORIZED"
		} else if strings.Contains(err.Error(), "validation") {
			status = http.StatusBadRequest
			code = "VALIDATION_FAILED"
		}
		h.writeError(w, status, code, err.Error(), start, meta.RequestID, meta.CorrelationID)
		return
	}

	w.WriteHeader(http.StatusOK)
	resp := types.ApiResponse{
		Success:    true,
		StatusCode: http.StatusOK,
		Data:       result,
		Meta: types.ResponseMeta{
			RequestID:       meta.RequestID,
			CorrelationID:   meta.CorrelationID,
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
			ExecutionTimeMs: time.Since(start).Milliseconds(),
			APIVersion:      "v1",
		},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *GitOpsReconciliationHandler) writeError(w http.ResponseWriter, status int, code string, msg string, start time.Time, reqID string, corrID string) {
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
