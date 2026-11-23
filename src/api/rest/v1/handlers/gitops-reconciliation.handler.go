/*
ALGORITHM & ARCHITECTURE BLUEPRINT: GITOPS RECONCILIATION REST HANDLER

1. Ingress Flow Execution:
  - Ingests W3C traceparent, correlation, tenant, and idempotency headers.
  - Decodes closed-schema JSON payload for POST /api/v1/gitops/sync.
  - Enforces Tier-2 handler spans ("gitops.handler.*") wrapping all endpoint executions.
  - Dispatches validated commands to GitOpsReconciliationService.
  - Formats success or typed error responses into ApiResponse envelopes.
  - Zero inline comments inside method bodies.
*/
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	gitopsreconciliation "internal-developer-platform/src/features/gitops-reconciliation"
	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	appErrors "internal-developer-platform/src/shared/errors"
	"internal-developer-platform/src/shared/types"
)

type GitOpsReconciliationHandler struct {
	service *gitopsreconciliation.Service
}

func NewGitOpsReconciliationHandler(s *gitopsreconciliation.Service) *GitOpsReconciliationHandler {
	return &GitOpsReconciliationHandler{service: s}
}

func (h *GitOpsReconciliationHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "gitops.handler.trigger_sync", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)
		tenantID := r.Header.Get(constants.HeaderXTenantID)
		idempotencyKey := r.Header.Get(constants.HeaderXIdempotencyKey)

		var cmd contracts.SyncCommand
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&cmd); err != nil {
			h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "malformed request payload", start, reqID, corrID)
			return err
		}

		if cmd.TenantID == "" && tenantID != "" {
			cmd.TenantID = tenantID
		}
		if cmd.IdempotencyKey == "" && idempotencyKey != "" {
			cmd.IdempotencyKey = idempotencyKey
		}

		result, err := h.service.TriggerReconciliation(ctx, cmd)
		if err != nil {
			if appErr, ok := err.(*appErrors.ApplicationError); ok {
				h.writeError(w, appErr.StatusCode, appErr.CanonicalCode, appErr.Message, start, reqID, corrID)
				return err
			}
			h.writeError(w, http.StatusInternalServerError, constants.ErrCodeInternalError, "internal error", start, reqID, corrID)
			return err
		}

		w.WriteHeader(http.StatusAccepted)
		if traceparent != "" {
			w.Header().Set(constants.HeaderTraceparent, traceparent)
		}

		resp := types.ApiResponse{
			Success:    true,
			StatusCode: http.StatusAccepted,
			Data:       result,
			Meta: types.ResponseMeta{
				RequestID:       reqID,
				CorrelationID:   corrID,
				Timestamp:       time.Now().UTC().Format(time.RFC3339),
				ExecutionTimeMs: time.Since(start).Milliseconds(),
				APIVersion:      "v1",
			},
		}
		return json.NewEncoder(w).Encode(resp)
	})
}

func (h *GitOpsReconciliationHandler) GetReconciliation(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "gitops.handler.get_reconciliation", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)
		tenantID := r.Header.Get(constants.HeaderXTenantID)

		pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(pathParts) < 4 {
			h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "missing reconciliation id in path", start, reqID, corrID)
			return nil
		}
		id := pathParts[len(pathParts)-1]

		result, err := h.service.GetReconciliation(ctx, tenantID, id)
		if err != nil {
			if appErr, ok := err.(*appErrors.ApplicationError); ok {
				h.writeError(w, appErr.StatusCode, appErr.CanonicalCode, appErr.Message, start, reqID, corrID)
				return err
			}
			h.writeError(w, http.StatusInternalServerError, constants.ErrCodeInternalError, "internal error", start, reqID, corrID)
			return err
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
		return json.NewEncoder(w).Encode(resp)
	})
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
