package restv1

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	gitopsService "internal-developer-platform/src/features/gitops-reconciliation/service"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	sharedErrors "internal-developer-platform/src/shared/errors"
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

func resolveErrorStatus(err error) (int, string) {
	var appErr *sharedErrors.ApplicationError
	if stdErrors.As(err, &appErr) {
		return appErr.StatusCode, appErr.CanonicalCode
	}
	msg := err.Error()
	if strings.Contains(msg, "validation") {
		return http.StatusBadRequest, "VALIDATION_FAILED"
	}
	if strings.Contains(msg, "signature") || strings.Contains(msg, "unauthorized") {
		return http.StatusUnauthorized, "UNAUTHORIZED"
	}
	if strings.Contains(msg, "prohibited") || strings.Contains(msg, "forbidden") {
		return http.StatusForbidden, "FORBIDDEN"
	}
	if strings.Contains(msg, "not found") {
		return http.StatusNotFound, "NOT_FOUND"
	}
	if strings.Contains(msg, "conflict") || strings.Contains(msg, "transition") {
		return http.StatusConflict, "CONFLICT"
	}
	return http.StatusInternalServerError, "INTERNAL_ERROR"
}

func (h *GitOpsReconciliationHandler) startHandlerSpan(w http.ResponseWriter, r *http.Request, spanName string, meta utils.IngressMetadata) (context.Context, func()) {
	ctx := observability.WithTenantID(r.Context(), meta.TenantID)
	ctx, span := observability.GetTracer().StartSpan(ctx, spanName,
		attribute.String("tenant.id", meta.TenantID),
		attribute.String("correlation.id", meta.CorrelationID),
		attribute.String("http.method", r.Method),
		attribute.String("http.target", r.URL.Path),
	)
	sc := span.SpanContext()
	if sc.IsValid() {
		w.Header().Set("X-Trace-Id", sc.TraceID().String())
		w.Header().Set("X-Span-Id", sc.SpanID().String())
		if meta.Traceparent == "" {
			w.Header().Set(constants.HeaderTraceparent, fmt.Sprintf("00-%s-%s-01", sc.TraceID().String(), sc.SpanID().String()))
		}
	}
	return ctx, func() { span.End() }
}

func (h *GitOpsReconciliationHandler) ConfigureSecretMechanism(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	meta := utils.ExtractIngressMetadata(r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)
	ctx, endSpan := h.startHandlerSpan(w, r, "gitops_reconciliation.configure_secret", meta)
	defer endSpan()

	var cfg contracts.SecretBackendConfig
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		h.writeError(ctx, w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid or unknown fields in json request body", start, meta.RequestID, meta.CorrelationID)
		return
	}
	if cfg.TenantID == "" {
		cfg.TenantID = meta.TenantID
	}

	saved, err := h.service.ConfigureSecretMechanism(ctx, cfg)
	if err != nil {
		h.writeError(ctx, w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), start, meta.RequestID, meta.CorrelationID)
		return
	}

	resp := types.ApiResponse{
		Success:    true,
		StatusCode: http.StatusOK,
		Data:       saved,
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

func (h *GitOpsReconciliationHandler) GetSecretMechanismConfig(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	meta := utils.ExtractIngressMetadata(r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)
	ctx, endSpan := h.startHandlerSpan(w, r, "gitops_reconciliation.get_secret_config", meta)
	defer endSpan()

	cfg, err := h.service.GetSecretMechanismConfig(ctx, meta.TenantID)
	if err != nil {
		h.writeError(ctx, w, http.StatusNotFound, "NOT_FOUND", err.Error(), start, meta.RequestID, meta.CorrelationID)
		return
	}

	resp := types.ApiResponse{
		Success:    true,
		StatusCode: http.StatusOK,
		Data:       cfg,
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

func (h *GitOpsReconciliationHandler) ValidateSecretManifest(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	meta := utils.ExtractIngressMetadata(r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)
	ctx, endSpan := h.startHandlerSpan(w, r, "gitops_reconciliation.validate_secret_manifest", meta)
	defer endSpan()

	var reqBody struct {
		Mechanism           contracts.SecretMechanismType `json:"mechanism"`
		HasPlaintextData    bool                          `json:"hasPlaintextData"`
		PluginInjectsValues bool                          `json:"pluginInjectsValues"`
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&reqBody); err != nil {
		h.writeError(ctx, w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid or unknown fields in json request body", start, meta.RequestID, meta.CorrelationID)
		return
	}

	result := h.service.ValidateSecretManifest(ctx, reqBody.Mechanism, reqBody.HasPlaintextData, reqBody.PluginInjectsValues)
	status := http.StatusOK
	if !result.Allowed {
		status = http.StatusForbidden
	}
	w.WriteHeader(status)
	resp := types.ApiResponse{
		Success:    result.Allowed,
		StatusCode: status,
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

func (h *GitOpsReconciliationHandler) ExecuteFleetRollout(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	meta := utils.ExtractIngressMetadata(r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)
	ctx, endSpan := h.startHandlerSpan(w, r, "gitops_reconciliation.fleet_rollout", meta)
	defer endSpan()

	var cmd contracts.FleetRolloutCommand
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cmd); err != nil {
		h.writeError(ctx, w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid or unknown fields in json request body", start, meta.RequestID, meta.CorrelationID)
		return
	}
	if cmd.TenantID == "" {
		cmd.TenantID = meta.TenantID
	}

	result, err := h.service.ExecuteFleetRingRollout(ctx, cmd)
	if err != nil {
		status, code := resolveErrorStatus(err)
		h.writeError(ctx, w, status, code, err.Error(), start, meta.RequestID, meta.CorrelationID)
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

func (h *GitOpsReconciliationHandler) TriggerSync(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	meta := utils.ExtractIngressMetadata(r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)
	ctx, endSpan := h.startHandlerSpan(w, r, "gitops_reconciliation.trigger_sync", meta)
	defer endSpan()

	var cmd contracts.SyncCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		h.writeError(ctx, w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid json request body", start, meta.RequestID, meta.CorrelationID)
		return
	}

	if cmd.ID == "" {
		cmd.ID = fmt.Sprintf("rec-%d", time.Now().UnixNano())
	}
	if cmd.TenantID == "" {
		cmd.TenantID = meta.TenantID
	}
	if cmd.IdempotencyKey == "" {
		cmd.IdempotencyKey = r.Header.Get(constants.HeaderXIdempotencyKey)
	}

	result, err := h.service.TriggerReconciliation(ctx, cmd)
	if err != nil {
		status, code := resolveErrorStatus(err)
		h.writeError(ctx, w, status, code, err.Error(), start, meta.RequestID, meta.CorrelationID)
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

func (h *GitOpsReconciliationHandler) ListReconciliations(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	meta := utils.ExtractIngressMetadata(r)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)
	ctx, endSpan := h.startHandlerSpan(w, r, "gitops_reconciliation.list_reconciliations", meta)
	defer endSpan()

	q := r.URL.Query()
	limitStr := q.Get("limit")
	offsetStr := q.Get("offset")
	limit := 50
	offset := 0
	if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
		limit = v
	}
	if v, err := strconv.Atoi(offsetStr); err == nil && v >= 0 {
		offset = v
	}

	filter := contracts.FilterCriteria{
		TenantID:  meta.TenantID,
		ServiceID: q.Get("serviceId"),
		SyncPhase: contracts.SyncPhase(q.Get("syncPhase")),
		Limit:     limit,
		Offset:    offset,
	}

	records, err := h.service.ListReconciliations(ctx, filter)
	if err != nil {
		status, code := resolveErrorStatus(err)
		h.writeError(ctx, w, status, code, err.Error(), start, meta.RequestID, meta.CorrelationID)
		return
	}

	w.WriteHeader(http.StatusOK)
	resp := types.ApiResponse{
		Success:    true,
		StatusCode: http.StatusOK,
		Data:       records,
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
	ctx, endSpan := h.startHandlerSpan(w, r, "gitops_reconciliation.get_reconciliation", meta)
	defer endSpan()

	path := strings.TrimPrefix(r.URL.Path, constants.EndpointReconciliationsPrefix)
	id := strings.TrimSuffix(path, "/diff")

	record, err := h.service.GetReconciliation(ctx, meta.TenantID, id)
	if err != nil || record == nil {
		h.writeError(ctx, w, http.StatusNotFound, "NOT_FOUND", "reconciliation not found", start, meta.RequestID, meta.CorrelationID)
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
	ctx, endSpan := h.startHandlerSpan(w, r, "gitops_reconciliation.get_drift_diff", meta)
	defer endSpan()

	path := strings.TrimPrefix(r.URL.Path, constants.EndpointReconciliationsPrefix)
	id := strings.TrimSuffix(path, "/diff")

	diffReport, err := h.service.InspectDrift(ctx, meta.TenantID, id)
	if err != nil {
		status := http.StatusInternalServerError
		code := "INTERNAL_ERROR"
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
			code = "NOT_FOUND"
		}
		h.writeError(ctx, w, status, code, err.Error(), start, meta.RequestID, meta.CorrelationID)
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
	ctx, endSpan := h.startHandlerSpan(w, r, "gitops_reconciliation.handle_webhook", meta)
	defer endSpan()

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(ctx, w, http.StatusBadRequest, "VALIDATION_FAILED", "failed to read webhook payload", start, meta.RequestID, meta.CorrelationID)
		return
	}

	var payload contracts.GitOpsWebhookPayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		h.writeError(ctx, w, http.StatusBadRequest, "VALIDATION_FAILED", "invalid json payload", start, meta.RequestID, meta.CorrelationID)
		return
	}

	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		signature = r.Header.Get("X-ArgoCD-Signature")
	}

	result, err := h.service.IngestWebhook(ctx, payload, signature, h.webhookSecret, bodyBytes)
	if err != nil {
		status, code := resolveErrorStatus(err)
		h.writeError(ctx, w, status, code, err.Error(), start, meta.RequestID, meta.CorrelationID)
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

func (h *GitOpsReconciliationHandler) writeError(ctx context.Context, w http.ResponseWriter, status int, code string, msg string, start time.Time, reqID string, corrID string) {
	span := observability.CurrentSpan(ctx)
	observability.RecordError(span, fmt.Errorf("%s: %s", code, msg))
	span.SetAttributes(
		attribute.String("error.type", code),
		attribute.Int("http.status_code", status),
	)

	logger := observability.GetLogger()
	logFields := []observability.Field{
		observability.String("error.type", code),
		observability.String("error", msg),
		observability.Int("http.status_code", status),
		observability.String("request_id", reqID),
		observability.String("correlation_id", corrID),
		observability.Int64("duration_ms", time.Since(start).Milliseconds()),
	}
	if status >= http.StatusInternalServerError {
		logger.Error(ctx, "gitops reconciliation handler error", logFields...)
	} else {
		logger.Warn(ctx, "gitops reconciliation client rejection", logFields...)
	}

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
