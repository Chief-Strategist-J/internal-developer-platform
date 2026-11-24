/*
ALGORITHM & ARCHITECTURE BLUEPRINT: POLICY-AS-CODE REST INGRESS HANDLER

1. Ingress Flow Execution:
   - Ingests W3C traceparent, correlation, causation, and tenant headers.
   - Enforces Tier-2 handler spans ("policy.handler.*") wrapping all endpoint executions.
   - Dispatches to PolicyAsCodeService under child span.
   - Returns typed ApiResponse envelopes with no inline comments inside function bodies.
   - Adheres strictly to 06-writing-conditions.md and 4-performance-and-durability.md.
*/
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"internal-developer-platform/src/features/policy-as-code/contracts"
	"internal-developer-platform/src/features/policy-as-code/domain/machines"
	"internal-developer-platform/src/features/policy-as-code/domain/workflows"
	"internal-developer-platform/src/features/policy-as-code/service"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	appErrors "internal-developer-platform/src/shared/errors"
	"internal-developer-platform/src/shared/types"
)

type PolicyAsCodeHandler struct {
	service *service.PolicyAsCodeService
}

func NewPolicyAsCodeHandler(s *service.PolicyAsCodeService) *PolicyAsCodeHandler {
	return &PolicyAsCodeHandler{service: s}
}

type LifecycleTransitionRequest struct {
	Event            string `json:"event"`
	ActiveViolations int    `json:"activeViolations"`
}

func (h *PolicyAsCodeHandler) CreatePolicy(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "policy.handler.create_policy", func(ctx context.Context) error {
		meta := extractIngressMetadata(r)
		var p contracts.PolicyRecord
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&p); err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), meta)
			return err
		}

		if (p.TenantID == "") && (meta.TenantID != "") {
			p.TenantID = meta.TenantID
		}
		if (p.CorrelationID == "") && (meta.CorrelationID != "") {
			p.CorrelationID = meta.CorrelationID
		}

		created, err := h.service.CreatePolicy(ctx, p)
		if err != nil {
			h.handleError(w, meta, err)
			return err
		}

		h.writeSuccessResponse(w, http.StatusCreated, created, meta)
		return nil
	})
}

func (h *PolicyAsCodeHandler) GetPolicy(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "policy.handler.get_policy", func(ctx context.Context) error {
		meta := extractIngressMetadata(r)
		tenantID := r.URL.Query().Get("tenantId")
		if tenantID == "" {
			tenantID = meta.TenantID
		}
		if tenantID == "" {
			err := appErrors.NewValidationError("tenantId query parameter or header is required")
			h.handleError(w, meta, err)
			return err
		}

		policyID, err := extractPolicyID(r.URL.Path)
		if err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), meta)
			return err
		}

		record, err := h.service.GetPolicy(ctx, tenantID, policyID)
		if err != nil {
			h.handleError(w, meta, err)
			return err
		}

		h.writeSuccessResponse(w, http.StatusOK, record, meta)
		return nil
	})
}

func (h *PolicyAsCodeHandler) ListActivePolicies(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "policy.handler.list_active", func(ctx context.Context) error {
		meta := extractIngressMetadata(r)
		tenantID := r.URL.Query().Get("tenantId")
		if tenantID == "" {
			tenantID = meta.TenantID
		}
		if tenantID == "" {
			err := appErrors.NewValidationError("tenantId query parameter or header is required")
			h.handleError(w, meta, err)
			return err
		}

		records, err := h.service.ListActivePolicies(ctx, tenantID)
		if err != nil {
			h.handleError(w, meta, err)
			return err
		}

		h.writeSuccessResponse(w, http.StatusOK, records, meta)
		return nil
	})
}

func (h *PolicyAsCodeHandler) TransitionLifecycle(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "policy.handler.transition_lifecycle", func(ctx context.Context) error {
		meta := extractIngressMetadata(r)
		tenantID := r.URL.Query().Get("tenantId")
		if tenantID == "" {
			tenantID = meta.TenantID
		}
		if tenantID == "" {
			err := appErrors.NewValidationError("tenantId is required")
			h.handleError(w, meta, err)
			return err
		}

		policyID, err := extractPolicySubID(r.URL.Path, "lifecycle")
		if err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), meta)
			return err
		}

		var req LifecycleTransitionRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), meta)
			return err
		}

		event := machines.PolicyLifecycleEvent(strings.ToUpper(strings.TrimSpace(req.Event)))
		updated, err := h.service.TransitionPolicyLifecycle(ctx, tenantID, policyID, event, req.ActiveViolations)
		if err != nil {
			h.handleError(w, meta, err)
			return err
		}

		h.writeSuccessResponse(w, http.StatusOK, updated, meta)
		return nil
	})
}

func (h *PolicyAsCodeHandler) RequestException(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "policy.handler.request_exception", func(ctx context.Context) error {
		meta := extractIngressMetadata(r)
		policyID, err := extractPolicySubID(r.URL.Path, "exceptions")
		if err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), meta)
			return err
		}

		var exc contracts.PolicyExceptionRecord
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&exc); err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), meta)
			return err
		}

		exc.PolicyID = policyID
		if (exc.TenantID == "") && (meta.TenantID != "") {
			exc.TenantID = meta.TenantID
		}

		saved, err := h.service.RequestException(ctx, exc)
		if err != nil {
			h.handleError(w, meta, err)
			return err
		}

		h.writeSuccessResponse(w, http.StatusCreated, saved, meta)
		return nil
	})
}

func (h *PolicyAsCodeHandler) EvaluateAdmission(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "policy.handler.evaluate_admission", func(ctx context.Context) error {
		meta := extractIngressMetadata(r)
		var input workflows.AdmissionEvaluationInput
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), meta)
			return err
		}

		if (input.TenantID == "") && (meta.TenantID != "") {
			input.TenantID = meta.TenantID
		}

		verdict, err := h.service.EvaluateAdmission(ctx, input)
		if err != nil {
			h.handleError(w, meta, err)
			return err
		}

		h.writeSuccessResponse(w, http.StatusOK, verdict, meta)
		return nil
	})
}

type ResolveViolationRequest struct {
	Version int64 `json:"version"`
}

func (h *PolicyAsCodeHandler) ListViolations(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "policy.handler.list_violations", func(ctx context.Context) error {
		meta := extractIngressMetadata(r)
		tenantID := r.URL.Query().Get("tenantId")
		if tenantID == "" {
			tenantID = meta.TenantID
		}
		if tenantID == "" {
			err := appErrors.NewValidationError("tenantId is required")
			h.handleError(w, meta, err)
			return err
		}

		policyID, err := extractPolicySubID(r.URL.Path, "violations")
		if err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), meta)
			return err
		}

		violations, err := h.service.ListActiveViolations(ctx, tenantID, policyID)
		if err != nil {
			h.handleError(w, meta, err)
			return err
		}

		h.writeSuccessResponse(w, http.StatusOK, violations, meta)
		return nil
	})
}

func (h *PolicyAsCodeHandler) ResolveViolation(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "policy.handler.resolve_violation", func(ctx context.Context) error {
		meta := extractIngressMetadata(r)
		tenantID := r.URL.Query().Get("tenantId")
		if tenantID == "" {
			tenantID = meta.TenantID
		}
		if tenantID == "" {
			err := appErrors.NewValidationError("tenantId is required")
			h.handleError(w, meta, err)
			return err
		}

		policyID, violationID, err := extractPolicyViolationResolveIDs(r.URL.Path)
		if err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), meta)
			return err
		}

		var req ResolveViolationRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), meta)
			return err
		}

		err = h.service.ResolveViolation(ctx, tenantID, violationID, req.Version)
		if err != nil {
			h.handleError(w, meta, err)
			return err
		}

		h.writeSuccessResponse(w, http.StatusOK, map[string]string{
			"policyId":    policyID,
			"violationId": violationID,
			"status":      "RESOLVED",
		}, meta)
		return nil
	})
}

func (h *PolicyAsCodeHandler) ExecuteAuditScan(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "policy.handler.execute_audit_scan", func(ctx context.Context) error {
		meta := extractIngressMetadata(r)
		var input workflows.AuditScanInput
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			h.writeErrorResponse(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), meta)
			return err
		}

		if (input.TenantID == "") && (meta.TenantID != "") {
			input.TenantID = meta.TenantID
		}

		report, err := h.service.ExecuteAuditScan(ctx, input)
		if err != nil {
			h.handleError(w, meta, err)
			return err
		}

		h.writeSuccessResponse(w, http.StatusOK, report, meta)
		return nil
	})
}

type ReconcileExceptionsResponse struct {
	ReconciledCount int `json:"reconciledCount"`
}

func (h *PolicyAsCodeHandler) ReconcileExceptions(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "policy.handler.reconcile_exceptions", func(ctx context.Context) error {
		meta := extractIngressMetadata(r)
		tenantID := r.URL.Query().Get("tenantId")
		if tenantID == "" {
			tenantID = meta.TenantID
		}
		if tenantID == "" {
			err := appErrors.NewValidationError("tenantId is required")
			h.handleError(w, meta, err)
			return err
		}

		count, err := h.service.ReconcileExpiredExceptions(ctx, tenantID)
		if err != nil {
			h.handleError(w, meta, err)
			return err
		}

		h.writeSuccessResponse(w, http.StatusOK, ReconcileExceptionsResponse{ReconciledCount: count}, meta)
		return nil
	})
}

func (h *PolicyAsCodeHandler) handleError(w http.ResponseWriter, meta IngressMetadata, err error) {
	var appErr *appErrors.ApplicationError
	if errors.As(err, &appErr) {
		h.writeErrorResponse(w, appErr.StatusCode, appErr.CanonicalCode, appErr.Message, meta)
		return
	}
	h.writeErrorResponse(w, http.StatusInternalServerError, constants.ErrCodeInternalError, "internal error", meta)
}

func (h *PolicyAsCodeHandler) writeSuccessResponse(w http.ResponseWriter, status int, data any, meta IngressMetadata) {
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

func (h *PolicyAsCodeHandler) writeErrorResponse(w http.ResponseWriter, status int, code, msg string, meta IngressMetadata) {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	if meta.Traceparent != "" {
		w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)
	}
	if meta.RequestID != "" {
		w.Header().Set(constants.HeaderXRequestID, meta.RequestID)
	}
	w.WriteHeader(status)
	resp := types.ApiResponse{
		Success:    false,
		StatusCode: status,
		Error:      &types.ResponseError{Code: code, Message: msg},
		Meta:       buildMeta(meta),
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func extractPolicyID(urlPath string) (string, error) {
	trimmed := strings.Trim(urlPath, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 4 {
		return "", errors.New("missing policy id in path")
	}
	id := parts[len(parts)-1]
	if id == "" {
		return "", errors.New("invalid policy id in path")
	}
	return id, nil
}

func extractPolicySubID(urlPath string, expectedSub string) (string, error) {
	trimmed := strings.Trim(urlPath, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 5 {
		return "", errors.New("missing policy id in path")
	}
	if parts[len(parts)-1] != expectedSub {
		return "", errors.New("invalid sub-resource path")
	}
	id := parts[len(parts)-2]
	if id == "" {
		return "", errors.New("invalid policy id in path")
	}
	return id, nil
}

func extractPolicyViolationResolveIDs(urlPath string) (string, string, error) {
	trimmed := strings.Trim(urlPath, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) < 7 {
		return "", "", errors.New("invalid violation resolve path")
	}
	if parts[len(parts)-1] != "resolve" || parts[len(parts)-3] != "violations" {
		return "", "", errors.New("invalid violation resolve path")
	}
	policyID := parts[len(parts)-4]
	violationID := parts[len(parts)-2]
	if policyID == "" || violationID == "" {
		return "", "", errors.New("missing policy or violation id in path")
	}
	return policyID, violationID, nil
}
