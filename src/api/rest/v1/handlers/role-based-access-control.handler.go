/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC REST HANDLER

1. Ingress Execution:
  - Ingests W3C traceparent, correlation, and tenant headers.
  - Decodes closed-schema JSON payload with disallowance of unknown fields.
  - Enforces Tier-2 handler spans ("rbac.handler.*") wrapping all endpoint executions.
  - Dispatches authenticated/authorized requests to RBACService.
  - Zero inline comments inside method bodies.
*/
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"internal-developer-platform/src/api/rest/v1/middleware"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	sharedErrors "internal-developer-platform/src/shared/errors"
	"internal-developer-platform/src/shared/types"
)

type RBACHandler struct {
	rbacSvc *rbacService.RBACService
}

func NewRBACHandler(s *rbacService.RBACService) *RBACHandler {
	return &RBACHandler{rbacSvc: s}
}

func (h *RBACHandler) IssueToken(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "rbac.handler.issue_token", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)

		var req rbacContracts.IssueTokenRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "malformed request payload", start, reqID, corrID)
			return err
		}

		if req.SubjectID == "" || req.TenantID == "" {
			h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "subjectId and tenantId are required", start, reqID, corrID)
			return nil
		}

		ttl := 2 * time.Hour
		if req.TTLSeconds > 0 {
			ttl = time.Duration(req.TTLSeconds) * time.Second
		}

		exp := time.Now().Add(ttl)
		sub := rbacContracts.Subject{
			ID:          req.SubjectID,
			TenantID:    req.TenantID,
			Roles:       req.Roles,
			Permissions: req.Permissions,
			ExpiresAt:   exp,
		}

		if len(sub.Roles) == 0 {
			if roles, err := h.rbacSvc.GetUserRoles(ctx, sub.TenantID, sub.ID); err == nil && len(roles) > 0 {
				sub.Roles = roles
			}
		}

		token, err := h.rbacSvc.IssueToken(sub)
		if err != nil {
			h.writeError(w, http.StatusInternalServerError, constants.ErrCodeInternalError, "failed to issue token", start, reqID, corrID)
			return err
		}

		respData := rbacContracts.IssueTokenResponse{
			AccessToken: token,
			TokenType:   "Bearer",
			ExpiresIn:   int64(ttl.Seconds()),
			SubjectID:   sub.ID,
			TenantID:    sub.TenantID,
			Roles:       sub.Roles,
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set(constants.HeaderTraceparent, traceparent)
		return json.NewEncoder(w).Encode(types.ApiResponse{
			Success:    true,
			StatusCode: http.StatusOK,
			Data:       respData,
			Meta: types.ResponseMeta{
				RequestID:       reqID,
				CorrelationID:   corrID,
				Timestamp:       time.Now().UTC().Format(time.RFC3339),
				ExecutionTimeMs: time.Since(start).Milliseconds(),
				APIVersion:      "v1",
			},
		})
	})
}

func (h *RBACHandler) GetCallerIdentity(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "rbac.handler.get_caller_identity", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)

		sub := middleware.GetSubjectFromContext(ctx)
		if sub == nil {
			h.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthenticated session", start, reqID, corrID)
			return nil
		}

		respData := rbacContracts.CallerIdentityResponse{
			SubjectID:   sub.ID,
			TenantID:    sub.TenantID,
			Roles:       sub.Roles,
			Permissions: sub.Permissions,
			Issuer:      sub.Issuer,
			ExpiresAt:   sub.ExpiresAt.UTC().Format(time.RFC3339),
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set(constants.HeaderTraceparent, traceparent)
		return json.NewEncoder(w).Encode(types.ApiResponse{
			Success:    true,
			StatusCode: http.StatusOK,
			Data:       respData,
			Meta: types.ResponseMeta{
				RequestID:       reqID,
				CorrelationID:   corrID,
				Timestamp:       time.Now().UTC().Format(time.RFC3339),
				ExecutionTimeMs: time.Since(start).Milliseconds(),
				APIVersion:      "v1",
			},
		})
	})
}

func (h *RBACHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "rbac.handler.revoke_session", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)

		var req rbacContracts.RevokeSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "malformed request payload", start, reqID, corrID)
			return err
		}

		if req.SubjectID == "" && req.JTI == "" {
			h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "either subjectId or jti must be provided", start, reqID, corrID)
			return nil
		}

		if req.SubjectID != "" {
			if err := h.rbacSvc.RevokeSubject(ctx, req.SubjectID); err != nil {
				h.writeError(w, http.StatusInternalServerError, constants.ErrCodeInternalError, "failed to revoke subject in revocation store", start, reqID, corrID)
				return err
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set(constants.HeaderTraceparent, traceparent)
		return json.NewEncoder(w).Encode(types.ApiResponse{
			Success:    true,
			StatusCode: http.StatusOK,
			Data:       map[string]string{"status": "revocation acknowledged"},
			Meta: types.ResponseMeta{
				RequestID:       reqID,
				CorrelationID:   corrID,
				Timestamp:       time.Now().UTC().Format(time.RFC3339),
				ExecutionTimeMs: time.Since(start).Milliseconds(),
				APIVersion:      "v1",
			},
		})
	})
}

func (h *RBACHandler) EvaluatePolicy(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "rbac.handler.evaluate_policy", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)

		sub := middleware.GetSubjectFromContext(ctx)
		if sub == nil {
			h.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthenticated", start, reqID, corrID)
			return nil
		}

		var req rbacContracts.EvaluatePolicyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "malformed request payload", start, reqID, corrID)
			return err
		}

		err := h.rbacSvc.Authorize(ctx, *sub, req.TargetTenantID, req.Resource, req.Action)
		granted := err == nil
		reason := "permission granted"
		if err != nil {
			reason = err.Error()
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set(constants.HeaderTraceparent, traceparent)
		return json.NewEncoder(w).Encode(types.ApiResponse{
			Success:    true,
			StatusCode: http.StatusOK,
			Data: rbacContracts.EvaluationResult{
				Granted: granted,
				Reason:  reason,
			},
			Meta: types.ResponseMeta{
				RequestID:       reqID,
				CorrelationID:   corrID,
				Timestamp:       time.Now().UTC().Format(time.RFC3339),
				ExecutionTimeMs: time.Since(start).Milliseconds(),
				APIVersion:      "v1",
			},
		})
	})
}

func (h *RBACHandler) writeError(w http.ResponseWriter, code int, errCode, message string, start time.Time, reqID, corrID string) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(types.ApiResponse{
		Success:    false,
		StatusCode: code,
		Error: &types.ResponseError{
			Code:      errCode,
			Message:   message,
			Retryable: code == http.StatusTooManyRequests || code == http.StatusServiceUnavailable,
		},
		Meta: types.ResponseMeta{
			RequestID:       reqID,
			CorrelationID:   corrID,
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
			ExecutionTimeMs: time.Since(start).Milliseconds(),
			APIVersion:      "v1",
		},
	})
}

func (h *RBACHandler) RequestJITAccess(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "rbac.handler.request_jit_access", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)

		var req rbacContracts.AccessGrantRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "malformed request payload", start, reqID, corrID)
			return err
		}

		result, err := h.rbacSvc.RequestJITAccess(ctx, req)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), start, reqID, corrID)
			return err
		}

		w.WriteHeader(http.StatusCreated)
		w.Header().Set(constants.HeaderTraceparent, traceparent)
		return json.NewEncoder(w).Encode(types.ApiResponse{
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
		})
	})
}

func (h *RBACHandler) ListActiveGrants(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "rbac.handler.list_active_grants", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)
		tenantID := r.Header.Get(constants.HeaderXTenantID)
		subjectID := r.URL.Query().Get("subjectId")

		grants, err := h.rbacSvc.ListActiveGrants(ctx, tenantID, subjectID)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), start, reqID, corrID)
			return err
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set(constants.HeaderTraceparent, traceparent)
		return json.NewEncoder(w).Encode(types.ApiResponse{
			Success:    true,
			StatusCode: http.StatusOK,
			Data:       grants,
			Meta: types.ResponseMeta{
				RequestID:       reqID,
				CorrelationID:   corrID,
				Timestamp:       time.Now().UTC().Format(time.RFC3339),
				ExecutionTimeMs: time.Since(start).Milliseconds(),
				APIVersion:      "v1",
			},
		})
	})
}

func (h *RBACHandler) RevokeJITAccess(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "rbac.handler.revoke_jit_access", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)
		tenantID := r.Header.Get(constants.HeaderXTenantID)

		trimmed := strings.Trim(r.URL.Path, "/")
		parts := strings.Split(trimmed, "/")
		grantID := ""
		if len(parts) >= 5 {
			grantID = parts[len(parts)-2]
		}

		if err := h.rbacSvc.RevokeJITAccess(ctx, tenantID, grantID); err != nil {
			h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), start, reqID, corrID)
			return err
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set(constants.HeaderTraceparent, traceparent)
		return json.NewEncoder(w).Encode(types.ApiResponse{
			Success:    true,
			StatusCode: http.StatusOK,
			Data:       map[string]string{"status": "grant revoked"},
			Meta: types.ResponseMeta{
				RequestID:       reqID,
				CorrelationID:   corrID,
				Timestamp:       time.Now().UTC().Format(time.RFC3339),
				ExecutionTimeMs: time.Since(start).Milliseconds(),
				APIVersion:      "v1",
			},
		})
	})
}

func (h *RBACHandler) BindUserRole(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "rbac.handler.bind_user_role", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)

		var req rbacContracts.BindUserRoleRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "malformed request payload", start, reqID, corrID)
			return err
		}

		if req.TenantID == "" {
			req.TenantID = r.Header.Get(constants.HeaderXTenantID)
		}

		record := rbacContracts.UserBindingRecord{
			SubjectID: req.SubjectID,
			TenantID:  req.TenantID,
			RoleName:  req.RoleName,
			CreatedAt: time.Now(),
		}

		if err := h.rbacSvc.BindUserRole(ctx, record); err != nil {
			statusCode := http.StatusBadRequest
			errCode := constants.ErrCodeValidationFailed
			if sharedErrors.IsConflictError(err) {
				statusCode = http.StatusConflict
				errCode = constants.ErrCodeConflict
			}
			h.writeError(w, statusCode, errCode, err.Error(), start, reqID, corrID)
			return err
		}

		w.WriteHeader(http.StatusCreated)
		w.Header().Set(constants.HeaderTraceparent, traceparent)
		return json.NewEncoder(w).Encode(types.ApiResponse{
			Success:    true,
			StatusCode: http.StatusCreated,
			Data:       record,
			Meta: types.ResponseMeta{
				RequestID:       reqID,
				CorrelationID:   corrID,
				Timestamp:       time.Now().UTC().Format(time.RFC3339),
				ExecutionTimeMs: time.Since(start).Milliseconds(),
				APIVersion:      "v1",
			},
		})
	})
}

func (h *RBACHandler) UnbindUserRole(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "rbac.handler.unbind_user_role", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)

		var req rbacContracts.UnbindUserRoleRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "malformed request payload", start, reqID, corrID)
			return err
		}

		if req.TenantID == "" {
			req.TenantID = r.Header.Get(constants.HeaderXTenantID)
		}

		if err := h.rbacSvc.UnbindUserRole(ctx, req.TenantID, req.SubjectID, req.RoleName); err != nil {
			statusCode := http.StatusBadRequest
			errCode := constants.ErrCodeValidationFailed
			if sharedErrors.IsNotFoundError(err) {
				statusCode = http.StatusNotFound
				errCode = constants.ErrCodeNotFound
			}
			h.writeError(w, statusCode, errCode, err.Error(), start, reqID, corrID)
			return err
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set(constants.HeaderTraceparent, traceparent)
		return json.NewEncoder(w).Encode(types.ApiResponse{
			Success:    true,
			StatusCode: http.StatusOK,
			Data:       map[string]string{"status": "user role unbound"},
			Meta: types.ResponseMeta{
				RequestID:       reqID,
				CorrelationID:   corrID,
				Timestamp:       time.Now().UTC().Format(time.RFC3339),
				ExecutionTimeMs: time.Since(start).Milliseconds(),
				APIVersion:      "v1",
			},
		})
	})
}

func (h *RBACHandler) GetUserRoles(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "rbac.handler.get_user_roles", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)
		tenantID := r.Header.Get(constants.HeaderXTenantID)
		subjectID := r.URL.Query().Get("subjectId")

		if subjectID == "" {
			trimmed := strings.Trim(r.URL.Path, "/")
			parts := strings.Split(trimmed, "/")
			if len(parts) >= 5 {
				subjectID = parts[len(parts)-1]
			}
		}

		roles, err := h.rbacSvc.GetUserRoles(ctx, tenantID, subjectID)
		if err != nil {
			h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error(), start, reqID, corrID)
			return err
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Set(constants.HeaderTraceparent, traceparent)
		return json.NewEncoder(w).Encode(types.ApiResponse{
			Success:    true,
			StatusCode: http.StatusOK,
			Data: rbacContracts.UserBindingsResponse{
				SubjectID: subjectID,
				TenantID:  tenantID,
				Roles:     roles,
			},
			Meta: types.ResponseMeta{
				RequestID:       reqID,
				CorrelationID:   corrID,
				Timestamp:       time.Now().UTC().Format(time.RFC3339),
				ExecutionTimeMs: time.Since(start).Milliseconds(),
				APIVersion:      "v1",
			},
		})
	})
}
