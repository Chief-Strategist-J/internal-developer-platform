package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"internal-developer-platform/src/api/rest/v1/middleware"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/types"
)

type RBACHandler struct {
	rbacSvc *rbacService.RBACService
}

func NewRBACHandler(s *rbacService.RBACService) *RBACHandler {
	return &RBACHandler{rbacSvc: s}
}

// IssueToken handles POST /api/v1/auth/token
func (h *RBACHandler) IssueToken(w http.ResponseWriter, r *http.Request) {
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
		return
	}

	if req.SubjectID == "" || req.TenantID == "" {
		h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "subjectId and tenantId are required", start, reqID, corrID)
		return
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

	token, err := h.rbacSvc.IssueToken(sub)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, constants.ErrCodeInternalError, "failed to issue token", start, reqID, corrID)
		return
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
	json.NewEncoder(w).Encode(types.ApiResponse{
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
}

// GetCallerIdentity handles GET /api/v1/auth/me
func (h *RBACHandler) GetCallerIdentity(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	w.Header().Set("Content-Type", "application/json")
	traceparent := r.Header.Get(constants.HeaderTraceparent)
	reqID := r.Header.Get(constants.HeaderXRequestID)
	corrID := r.Header.Get(constants.HeaderXCorrelationID)

	sub := middleware.GetSubjectFromContext(r.Context())
	if sub == nil {
		h.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthenticated session", start, reqID, corrID)
		return
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
	json.NewEncoder(w).Encode(types.ApiResponse{
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
}

// RevokeSession handles POST /api/v1/auth/revoke
func (h *RBACHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	w.Header().Set("Content-Type", "application/json")
	traceparent := r.Header.Get(constants.HeaderTraceparent)
	reqID := r.Header.Get(constants.HeaderXRequestID)
	corrID := r.Header.Get(constants.HeaderXCorrelationID)

	var req rbacContracts.RevokeSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "malformed request payload", start, reqID, corrID)
		return
	}

	if req.SubjectID == "" && req.JTI == "" {
		h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "either subjectId or jti must be provided", start, reqID, corrID)
		return
	}

	if req.SubjectID != "" {
		if err := h.rbacSvc.RevokeSubject(r.Context(), req.SubjectID); err != nil {
			h.writeError(w, http.StatusInternalServerError, constants.ErrCodeInternalError, "failed to revoke subject in revocation store", start, reqID, corrID)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set(constants.HeaderTraceparent, traceparent)
	json.NewEncoder(w).Encode(types.ApiResponse{
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
}

// EvaluatePolicy handles POST /api/v1/auth/evaluate (PDP endpoint)
func (h *RBACHandler) EvaluatePolicy(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	w.Header().Set("Content-Type", "application/json")
	traceparent := r.Header.Get(constants.HeaderTraceparent)
	reqID := r.Header.Get(constants.HeaderXRequestID)
	corrID := r.Header.Get(constants.HeaderXCorrelationID)

	sub := middleware.GetSubjectFromContext(r.Context())
	if sub == nil {
		h.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "unauthenticated", start, reqID, corrID)
		return
	}

	var req rbacContracts.EvaluatePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "malformed request payload", start, reqID, corrID)
		return
	}

	err := h.rbacSvc.Authorize(r.Context(), *sub, req.TargetTenantID, req.Resource, req.Action)
	granted := err == nil
	reason := "permission granted"
	if err != nil {
		reason = err.Error()
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set(constants.HeaderTraceparent, traceparent)
	json.NewEncoder(w).Encode(types.ApiResponse{
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
}

func (h *RBACHandler) writeError(w http.ResponseWriter, code int, errCode, message string, start time.Time, reqID, corrID string) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(types.ApiResponse{
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
