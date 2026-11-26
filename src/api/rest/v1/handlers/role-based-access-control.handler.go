/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC REST HANDLER (feature-files.md §2.1 COMPLIANT)

1. Public Functions:
  - IssueToken, GetCallerIdentity, RevokeSession, EvaluatePolicy, Authorize,
    ValidateRoleHierarchy, RequestJITAccess, ListActiveGrants, RevokeJITAccess,
    BindUserRole, UnbindUserRole, GetUserRoles.
  - Every handler follows: withSpan -> decode -> schema validate -> dispatch service -> emitSuccess/emitFailure -> completeRequest.

2. Private Functions (grouped by purpose):
  - decoding: decodeClosedJSON, extractSubjectIDFromRequest, extractGrantIDFromRequest
  - encoding: emitSuccess, emitFailure, buildResponseMeta
  - completion: completeRequest

3. Invariants:
  - Zero business logic in handler.
  - Zero mid-function inline comments.
  - Headers set before WriteHeader.
*/
package handlers

import (
	"context"
	"encoding/json"
	stdErrors "errors"
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

const rbacHandlerSpanPrefix = "rbac.handler"

type RBACHandler struct {
	rbacSvc *rbacService.RBACService
}

func NewRBACHandler(s *rbacService.RBACService) *RBACHandler {
	return &RBACHandler{rbacSvc: s}
}

func (h *RBACHandler) IssueToken(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), rbacHandlerSpanPrefix+".issue_token", func(ctx context.Context) error {
		start := time.Now()
		var req rbacContracts.IssueTokenRequest
		if err := h.decodeClosedJSON(r, &req); err != nil {
			h.emitFailure(w, r, sharedErrors.NewValidationError("malformed request payload"), start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		if err := rbacContracts.ValidateIssueTokenInput(req); err != nil {
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		ttl := 2 * time.Hour
		if req.TTLSeconds > 0 {
			ttl = time.Duration(req.TTLSeconds) * time.Second
		}
		sub := rbacContracts.SubjectFromIssueTokenAPI(req, time.Now().Add(ttl))
		if len(sub.Roles) == 0 {
			if roles, err := h.rbacSvc.GetUserRoles(ctx, sub.TenantID, sub.ID); err == nil && len(roles) > 0 {
				sub.Roles = roles
			}
		}
		token, err := h.rbacSvc.IssueToken(sub)
		if err != nil {
			h.emitFailure(w, r, sharedErrors.NewInternalError("failed to issue token", err), start)
			h.completeRequest(ctx, r.URL.Path, http.StatusInternalServerError, err)
			return err
		}
		respData := rbacContracts.IssueTokenResponse{
			AccessToken: token,
			TokenType:   strings.TrimSpace(constants.BearerPrefix),
			ExpiresIn:   int64(ttl.Seconds()),
			SubjectID:   sub.ID,
			TenantID:    sub.TenantID,
			Roles:       sub.Roles,
		}
		h.emitSuccess(w, r, http.StatusOK, respData, start)
		h.completeRequest(ctx, r.URL.Path, http.StatusOK, nil)
		return nil
	})
}

func (h *RBACHandler) GetCallerIdentity(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), rbacHandlerSpanPrefix+".get_caller_identity", func(ctx context.Context) error {
		start := time.Now()
		sub := middleware.GetSubjectFromContext(ctx)
		if sub == nil {
			err := sharedErrors.NewUnauthorizedError("unauthenticated session")
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusUnauthorized, err)
			return err
		}
		h.emitSuccess(w, r, http.StatusOK, rbacContracts.CallerIdentityToAPI(*sub), start)
		h.completeRequest(ctx, r.URL.Path, http.StatusOK, nil)
		return nil
	})
}

func (h *RBACHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), rbacHandlerSpanPrefix+".revoke_session", func(ctx context.Context) error {
		start := time.Now()
		var req rbacContracts.RevokeSessionRequest
		if err := h.decodeClosedJSON(r, &req); err != nil {
			h.emitFailure(w, r, sharedErrors.NewValidationError("malformed request payload"), start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		if err := rbacContracts.ValidateRevokeSessionInput(req); err != nil {
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		if req.SubjectID != "" {
			if err := h.rbacSvc.RevokeSubject(ctx, req.SubjectID); err != nil {
				h.emitFailure(w, r, sharedErrors.NewInternalError("failed to revoke subject", err), start)
				h.completeRequest(ctx, r.URL.Path, http.StatusInternalServerError, err)
				return err
			}
		}
		if req.JTI != "" {
			if err := h.rbacSvc.RevokeJTI(ctx, req.JTI); err != nil {
				h.emitFailure(w, r, sharedErrors.NewInternalError("failed to revoke jti", err), start)
				h.completeRequest(ctx, r.URL.Path, http.StatusInternalServerError, err)
				return err
			}
		}
		h.emitSuccess(w, r, http.StatusOK, map[string]string{"status": "revocation acknowledged"}, start)
		h.completeRequest(ctx, r.URL.Path, http.StatusOK, nil)
		return nil
	})
}

func (h *RBACHandler) EvaluatePolicy(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), rbacHandlerSpanPrefix+".evaluate_policy", func(ctx context.Context) error {
		start := time.Now()
		sub := middleware.GetSubjectFromContext(ctx)
		if sub == nil {
			err := sharedErrors.NewUnauthorizedError("unauthenticated")
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusUnauthorized, err)
			return err
		}
		var req rbacContracts.EvaluatePolicyRequest
		if err := h.decodeClosedJSON(r, &req); err != nil {
			h.emitFailure(w, r, sharedErrors.NewValidationError("malformed request payload"), start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		err := h.rbacSvc.Authorize(ctx, *sub, req.TargetTenantID, req.Resource, req.Action)
		granted := err == nil
		reason := "permission granted"
		if err != nil {
			reason = err.Error()
		}
		h.emitSuccess(w, r, http.StatusOK, rbacContracts.EvaluationResult{Granted: granted, Reason: reason}, start)
		h.completeRequest(ctx, r.URL.Path, http.StatusOK, nil)
		return nil
	})
}

func (h *RBACHandler) Authorize(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), rbacHandlerSpanPrefix+".authorize", func(ctx context.Context) error {
		start := time.Now()
		sub := middleware.GetSubjectFromContext(ctx)
		if sub == nil {
			err := sharedErrors.NewUnauthorizedError("unauthenticated")
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusUnauthorized, err)
			return err
		}
		var req rbacContracts.EvaluatePolicyRequest
		if err := h.decodeClosedJSON(r, &req); err != nil {
			h.emitFailure(w, r, sharedErrors.NewValidationError("malformed request payload"), start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		if err := rbacContracts.ValidateEvaluatePolicyInput(req); err != nil {
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		evalReq := rbacContracts.EvaluationRequestFromAPI(*sub, req)
		decision, err := h.rbacSvc.AuthorizeWithContext(ctx, evalReq)
		if err != nil {
			h.emitDecisionDenied(w, r, decision, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusForbidden, err)
			return err
		}
		h.emitSuccess(w, r, http.StatusOK, decision, start)
		h.completeRequest(ctx, r.URL.Path, http.StatusOK, nil)
		return nil
	})
}

func (h *RBACHandler) ValidateRoleHierarchy(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), rbacHandlerSpanPrefix+".validate_role_hierarchy", func(ctx context.Context) error {
		start := time.Now()
		var req rbacContracts.ValidateRoleHierarchyRequest
		if err := h.decodeClosedJSON(r, &req); err != nil {
			h.emitFailure(w, r, sharedErrors.NewValidationError("malformed request payload"), start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		if err := h.rbacSvc.ValidateRoleHierarchy(ctx, req.Edges); err != nil {
			h.emitFailureWithStatus(w, r, http.StatusUnprocessableEntity, constants.ErrCodeValidationFailed, err.Error(), start)
			h.completeRequest(ctx, r.URL.Path, http.StatusUnprocessableEntity, err)
			return err
		}
		h.emitSuccess(w, r, http.StatusOK, rbacContracts.ValidateRoleHierarchyResponse{Valid: true, Message: "role hierarchy DAG is valid"}, start)
		h.completeRequest(ctx, r.URL.Path, http.StatusOK, nil)
		return nil
	})
}

func (h *RBACHandler) RequestJITAccess(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), rbacHandlerSpanPrefix+".request_jit_access", func(ctx context.Context) error {
		start := time.Now()
		var req rbacContracts.AccessGrantRequest
		if err := h.decodeClosedJSON(r, &req); err != nil {
			h.emitFailure(w, r, sharedErrors.NewValidationError("malformed request payload"), start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		result, err := h.rbacSvc.RequestJITAccess(ctx, req)
		if err != nil {
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		h.emitSuccess(w, r, http.StatusCreated, result, start)
		h.completeRequest(ctx, r.URL.Path, http.StatusCreated, nil)
		return nil
	})
}

func (h *RBACHandler) ListActiveGrants(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), rbacHandlerSpanPrefix+".list_active_grants", func(ctx context.Context) error {
		start := time.Now()
		tenantID := r.Header.Get(constants.HeaderXTenantID)
		subjectID := r.URL.Query().Get("subjectId")
		grants, err := h.rbacSvc.ListActiveGrants(ctx, tenantID, subjectID)
		if err != nil {
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		h.emitSuccess(w, r, http.StatusOK, grants, start)
		h.completeRequest(ctx, r.URL.Path, http.StatusOK, nil)
		return nil
	})
}

func (h *RBACHandler) RevokeJITAccess(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), rbacHandlerSpanPrefix+".revoke_jit_access", func(ctx context.Context) error {
		start := time.Now()
		tenantID := r.Header.Get(constants.HeaderXTenantID)
		grantID := extractGrantIDFromRequest(r)
		if err := h.rbacSvc.RevokeJITAccess(ctx, tenantID, grantID); err != nil {
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		h.emitSuccess(w, r, http.StatusOK, map[string]string{"status": "grant revoked"}, start)
		h.completeRequest(ctx, r.URL.Path, http.StatusOK, nil)
		return nil
	})
}

func (h *RBACHandler) BindUserRole(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), rbacHandlerSpanPrefix+".bind_user_role", func(ctx context.Context) error {
		start := time.Now()
		var req rbacContracts.BindUserRoleRequest
		if err := h.decodeClosedJSON(r, &req); err != nil {
			h.emitFailure(w, r, sharedErrors.NewValidationError("malformed request payload"), start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		if req.TenantID == "" {
			req.TenantID = r.Header.Get(constants.HeaderXTenantID)
		}
		if err := rbacContracts.ValidateBindUserRoleInput(req); err != nil {
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		record := rbacContracts.UserBindingRecord{
			SubjectID: req.SubjectID,
			TenantID:  req.TenantID,
			RoleName:  req.RoleName,
			CreatedAt: time.Now().UTC(),
		}
		if err := h.rbacSvc.BindUserRole(ctx, record); err != nil {
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusConflict, err)
			return err
		}
		h.emitSuccess(w, r, http.StatusCreated, record, start)
		h.completeRequest(ctx, r.URL.Path, http.StatusCreated, nil)
		return nil
	})
}

func (h *RBACHandler) UnbindUserRole(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), rbacHandlerSpanPrefix+".unbind_user_role", func(ctx context.Context) error {
		start := time.Now()
		var req rbacContracts.UnbindUserRoleRequest
		if err := h.decodeClosedJSON(r, &req); err != nil {
			h.emitFailure(w, r, sharedErrors.NewValidationError("malformed request payload"), start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		if req.TenantID == "" {
			req.TenantID = r.Header.Get(constants.HeaderXTenantID)
		}
		if err := rbacContracts.ValidateUnbindUserRoleInput(req); err != nil {
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		if err := h.rbacSvc.UnbindUserRole(ctx, req.TenantID, req.SubjectID, req.RoleName); err != nil {
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusNotFound, err)
			return err
		}
		h.emitSuccess(w, r, http.StatusOK, map[string]string{"status": "user role unbound"}, start)
		h.completeRequest(ctx, r.URL.Path, http.StatusOK, nil)
		return nil
	})
}

func (h *RBACHandler) GetUserRoles(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), rbacHandlerSpanPrefix+".get_user_roles", func(ctx context.Context) error {
		start := time.Now()
		tenantID := r.Header.Get(constants.HeaderXTenantID)
		subjectID := extractSubjectIDFromRequest(r)
		roles, err := h.rbacSvc.GetUserRoles(ctx, tenantID, subjectID)
		if err != nil {
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		h.emitSuccess(w, r, http.StatusOK, rbacContracts.UserBindingsResponse{SubjectID: subjectID, TenantID: tenantID, Roles: roles}, start)
		h.completeRequest(ctx, r.URL.Path, http.StatusOK, nil)
		return nil
	})
}

func (h *RBACHandler) ClaimExclusiveRoleBinding(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), rbacHandlerSpanPrefix+".claim_exclusive_role_binding", func(ctx context.Context) error {
		start := time.Now()
		var req rbacContracts.BindUserRoleRequest
		if err := h.decodeClosedJSON(r, &req); err != nil {
			h.emitFailure(w, r, sharedErrors.NewValidationError("malformed request payload"), start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		if req.TenantID == "" {
			req.TenantID = r.Header.Get(constants.HeaderXTenantID)
		}
		if err := rbacContracts.ValidateBindUserRoleInput(req); err != nil {
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusBadRequest, err)
			return err
		}
		if err := h.rbacSvc.ClaimExclusiveRoleBinding(ctx, req.TenantID, req.SubjectID, req.RoleName); err != nil {
			h.emitFailure(w, r, err, start)
			h.completeRequest(ctx, r.URL.Path, http.StatusConflict, err)
			return err
		}
		h.emitSuccess(w, r, http.StatusOK, map[string]string{"status": "exclusive role binding claimed"}, start)
		h.completeRequest(ctx, r.URL.Path, http.StatusOK, nil)
		return nil
	})
}

func (h *RBACHandler) decodeClosedJSON(r *http.Request, dst interface{}) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func extractSubjectIDFromRequest(r *http.Request) string {
	subjectID := r.URL.Query().Get("subjectId")
	if subjectID != "" {
		return subjectID
	}
	trimmed := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) >= 5 {
		return parts[len(parts)-1]
	}
	return ""
}

func extractGrantIDFromRequest(r *http.Request) string {
	trimmed := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) >= 5 {
		return parts[len(parts)-2]
	}
	return ""
}

func (h *RBACHandler) emitSuccess(w http.ResponseWriter, r *http.Request, status int, data interface{}, start time.Time) {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.Header().Set(constants.HeaderTraceparent, r.Header.Get(constants.HeaderTraceparent))
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(types.ApiResponse{
		Success:    true,
		StatusCode: status,
		Data:       data,
		Meta:       buildResponseMeta(r, start),
	})
}

func (h *RBACHandler) emitDecisionDenied(w http.ResponseWriter, r *http.Request, decision rbacContracts.EvaluationResult, start time.Time) {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.Header().Set(constants.HeaderTraceparent, r.Header.Get(constants.HeaderTraceparent))
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(types.ApiResponse{
		Success:    false,
		StatusCode: http.StatusForbidden,
		Data:       decision,
		Error: &types.ResponseError{
			Code:    constants.ErrCodeForbidden,
			Message: decision.Reason,
		},
		Meta: buildResponseMeta(r, start),
	})
}

func (h *RBACHandler) emitFailure(w http.ResponseWriter, r *http.Request, err error, start time.Time) {
	statusCode := http.StatusInternalServerError
	errCode := constants.ErrCodeInternalError
	retryable := false
	message := "internal server error"
	var appErr *sharedErrors.ApplicationError
	if stdErrors.As(err, &appErr) {
		statusCode = appErr.StatusCode
		errCode = appErr.CanonicalCode
		retryable = appErr.Retryable
		message = appErr.Message
	} else if err != nil {
		statusCode = http.StatusBadRequest
		errCode = constants.ErrCodeValidationFailed
		message = err.Error()
	}
	h.emitFailureWithStatusAndRetry(w, r, statusCode, errCode, message, retryable, start)
}

func (h *RBACHandler) emitFailureWithStatus(w http.ResponseWriter, r *http.Request, status int, errCode string, message string, start time.Time) {
	h.emitFailureWithStatusAndRetry(w, r, status, errCode, message, status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable, start)
}

func (h *RBACHandler) emitFailureWithStatusAndRetry(w http.ResponseWriter, r *http.Request, status int, errCode string, message string, retryable bool, start time.Time) {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.Header().Set(constants.HeaderTraceparent, r.Header.Get(constants.HeaderTraceparent))
	if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
		w.Header().Set(constants.HeaderRetryAfter, "1")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(types.ApiResponse{
		Success:    false,
		StatusCode: status,
		Error: &types.ResponseError{
			Code:      errCode,
			Message:   message,
			Retryable: retryable,
		},
		Meta: buildResponseMeta(r, start),
	})
}

func buildResponseMeta(r *http.Request, start time.Time) types.ResponseMeta {
	return types.ResponseMeta{
		RequestID:       r.Header.Get(constants.HeaderXRequestID),
		CorrelationID:   r.Header.Get(constants.HeaderXCorrelationID),
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		ExecutionTimeMs: time.Since(start).Milliseconds(),
		APIVersion:      "v1",
	}
}

func (h *RBACHandler) completeRequest(ctx context.Context, route string, status int, err error) {
	if err != nil {
		observability.GetLogger().Warn(ctx, "rbac http request failed",
			observability.String("route", route),
			observability.Int("status", status),
			observability.String("error", err.Error()),
		)
		return
	}
	observability.GetLogger().Info(ctx, "rbac http request completed",
		observability.String("route", route),
		observability.Int("status", status),
	)
}

func (h *RBACHandler) EvaluateSidecarPDP(w http.ResponseWriter, r *http.Request) {
	start := time.Now().UTC()
	ctx, span := observability.GetTracer().StartSpan(r.Context(), "RBACHandler.EvaluateSidecarPDP")
	defer span.End()

	var req rbacContracts.EvaluationRequest
	if err := h.decodeClosedJSON(r, &req); err != nil {
		h.emitFailure(w, r, sharedErrors.NewValidationError(err.Error()), start)
		h.completeRequest(ctx, constants.EndpointRBACSidecarEvaluate, http.StatusBadRequest, err)
		return
	}

	decision, err := h.rbacSvc.EvaluateWithSidecarPDP(ctx, req)
	if err != nil {
		h.emitFailure(w, r, err, start)
		h.completeRequest(ctx, constants.EndpointRBACSidecarEvaluate, http.StatusInternalServerError, err)
		return
	}
	if !decision.Granted {
		h.emitDecisionDenied(w, r, decision, start)
		h.completeRequest(ctx, constants.EndpointRBACSidecarEvaluate, http.StatusForbidden, nil)
		return
	}
	h.emitSuccess(w, r, http.StatusOK, decision, start)
	h.completeRequest(ctx, constants.EndpointRBACSidecarEvaluate, http.StatusOK, nil)
}

type leastPrivilegeAnalyzeRequest struct {
	Roles      map[rbacContracts.RoleName][]rbacContracts.Permission `json:"roles"`
	Usage      []rbacContracts.PermissionUsageRecord                 `json:"usage"`
	StaleHours int                                                   `json:"staleHours"`
}

func (h *RBACHandler) AnalyzeLeastPrivilege(w http.ResponseWriter, r *http.Request) {
	start := time.Now().UTC()
	ctx, span := observability.GetTracer().StartSpan(r.Context(), "RBACHandler.AnalyzeLeastPrivilege")
	defer span.End()

	var req leastPrivilegeAnalyzeRequest
	if err := h.decodeClosedJSON(r, &req); err != nil {
		h.emitFailure(w, r, sharedErrors.NewValidationError(err.Error()), start)
		h.completeRequest(ctx, constants.EndpointRBACLeastPrivilege, http.StatusBadRequest, err)
		return
	}

	staleWindow := time.Duration(req.StaleHours) * time.Hour
	if staleWindow <= 0 {
		staleWindow = 24 * time.Hour
	}
	report := h.rbacSvc.AnalyzeLeastPrivilege(req.Roles, req.Usage, staleWindow)
	h.emitSuccess(w, r, http.StatusOK, report, start)
	h.completeRequest(ctx, constants.EndpointRBACLeastPrivilege, http.StatusOK, nil)
}
