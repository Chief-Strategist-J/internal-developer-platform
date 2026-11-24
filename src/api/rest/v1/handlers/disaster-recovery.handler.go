/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DISASTER RECOVERY INGRESS HANDLER

1. Ingress Flow Execution:
  - Ingests W3C trace context, correlation, causation, tenant ID, and idempotency key via shared utils.
  - Wraps execution in OpenTelemetry Tier-2 handler spans ("disaster_recovery.handler.*").
  - Enforces closed-schema JSON request validation (additionalProperties: false).
  - Routes to DisasterRecoveryService methods:
      * Plans: CreatePlan, GetPlan, ListPlans, CheckReadiness.
      * Incidents: DeclareIncident, ApproveIncident.
      * Failover: InitiateFailover, ApproveAndExecuteFailover, AbortFailover.
      * Failback: ExecuteFailback.
      * Fencing: FenceDataset, GetFencingEpoch.
      * Drills: RunDrill.
  - Encapsulates results in standard RFC-compliant ApiResponse envelopes.

2. Invariants:
  - Zero mid-function comments.
  - All responses return strictly typecast envelope payloads.
  - 100% comment-free function bodies.
*/
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	disasterrecovery "internal-developer-platform/src/features/disaster-recovery"
	"internal-developer-platform/src/features/disaster-recovery/contracts"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	appErrors "internal-developer-platform/src/shared/errors"
	"internal-developer-platform/src/shared/types"
	"internal-developer-platform/src/shared/utils"
)

type DisasterRecoveryHandler struct {
	service *disasterrecovery.Service
}

func NewDisasterRecoveryHandler(s *disasterrecovery.Service) *DisasterRecoveryHandler {
	return &DisasterRecoveryHandler{service: s}
}

func (h *DisasterRecoveryHandler) HandlePlans(w http.ResponseWriter, r *http.Request) {
	meta := utils.ExtractIngressMetadata(r)
	_ = observability.WithSpan(r.Context(), "disaster_recovery.handler.plans", func(ctx context.Context) error {
		switch r.Method {
		case http.MethodPost:
			h.createPlan(w, r, ctx, meta)
		case http.MethodGet:
			h.listPlans(w, r, ctx, meta)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
		return nil
	})
}

func (h *DisasterRecoveryHandler) HandleSinglePlan(w http.ResponseWriter, r *http.Request) {
	meta := utils.ExtractIngressMetadata(r)
	_ = observability.WithSpan(r.Context(), "disaster_recovery.handler.single_plan", func(ctx context.Context) error {
		path := strings.TrimPrefix(r.URL.Path, constants.EndpointDRPlansPrefix)

		if strings.HasSuffix(path, ":checkReadiness") && r.Method == http.MethodPost {
			planID := strings.TrimSuffix(path, ":checkReadiness")
			h.checkReadiness(w, r, ctx, meta, planID)
			return nil
		}

		if r.Method == http.MethodGet {
			h.getPlan(w, r, ctx, meta, path)
			return nil
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
		return nil
	})
}

func (h *DisasterRecoveryHandler) HandleDeclareIncident(w http.ResponseWriter, r *http.Request) {
	meta := utils.ExtractIngressMetadata(r)
	_ = observability.WithSpan(r.Context(), "disaster_recovery.handler.declare_incident", func(ctx context.Context) error {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return nil
		}

		var cmd contracts.DeclareIncidentCommand
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cmd); err != nil {
			h.writeError(w, meta, appErrors.NewValidationError("invalid request body schema"))
			return err
		}

		if cmd.TenantID == "" {
			cmd.TenantID = meta.TenantID
		}

		inc, err := h.service.DeclareIncident(ctx, cmd)
		if err != nil {
			h.writeError(w, meta, err)
			return err
		}

		h.writeSuccess(w, meta, http.StatusAccepted, inc)
		return nil
	})
}

func (h *DisasterRecoveryHandler) HandleApproveIncident(w http.ResponseWriter, r *http.Request) {
	meta := utils.ExtractIngressMetadata(r)
	_ = observability.WithSpan(r.Context(), "disaster_recovery.handler.approve_incident", func(ctx context.Context) error {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return nil
		}

		path := strings.TrimPrefix(r.URL.Path, constants.EndpointDRIncidentsApprovePrefix)
		incidentID := strings.TrimSuffix(path, ":approve")

		var cmd contracts.ApproveIncidentCommand
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cmd); err != nil {
			h.writeError(w, meta, appErrors.NewValidationError("invalid request body schema"))
			return err
		}

		cmd.IncidentID = incidentID
		if cmd.TenantID == "" {
			cmd.TenantID = meta.TenantID
		}

		inc, err := h.service.ApproveIncident(ctx, cmd)
		if err != nil {
			h.writeError(w, meta, err)
			return err
		}

		h.writeSuccess(w, meta, http.StatusOK, inc)
		return nil
	})
}

func (h *DisasterRecoveryHandler) HandleFailovers(w http.ResponseWriter, r *http.Request) {
	meta := utils.ExtractIngressMetadata(r)
	_ = observability.WithSpan(r.Context(), "disaster_recovery.handler.failovers", func(ctx context.Context) error {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return nil
		}

		var cmd contracts.InitiateFailoverCommand
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cmd); err != nil {
			h.writeError(w, meta, appErrors.NewValidationError("invalid request body schema"))
			return err
		}

		if cmd.TenantID == "" {
			cmd.TenantID = meta.TenantID
		}

		op, err := h.service.InitiateFailover(ctx, cmd)
		if err != nil {
			h.writeError(w, meta, err)
			return err
		}

		h.writeSuccess(w, meta, http.StatusAccepted, op)
		return nil
	})
}

func (h *DisasterRecoveryHandler) HandleSingleFailover(w http.ResponseWriter, r *http.Request) {
	meta := utils.ExtractIngressMetadata(r)
	_ = observability.WithSpan(r.Context(), "disaster_recovery.handler.single_failover", func(ctx context.Context) error {
		path := strings.TrimPrefix(r.URL.Path, constants.EndpointDRFailoversPrefix)

		if strings.HasSuffix(path, ":approve") && r.Method == http.MethodPost {
			opID := strings.TrimSuffix(path, ":approve")
			h.approveFailover(w, r, ctx, meta, opID)
			return nil
		}

		if strings.HasSuffix(path, ":abort") && r.Method == http.MethodPost {
			opID := strings.TrimSuffix(path, ":abort")
			h.abortFailover(w, r, ctx, meta, opID)
			return nil
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
		return nil
	})
}

func (h *DisasterRecoveryHandler) HandleFailback(w http.ResponseWriter, r *http.Request) {
	meta := utils.ExtractIngressMetadata(r)
	_ = observability.WithSpan(r.Context(), "disaster_recovery.handler.failback", func(ctx context.Context) error {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return nil
		}

		var cmd contracts.ExecuteFailbackCommand
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cmd); err != nil {
			h.writeError(w, meta, appErrors.NewValidationError("invalid request body schema"))
			return err
		}

		if cmd.TenantID == "" {
			cmd.TenantID = meta.TenantID
		}

		op, err := h.service.ExecuteFailback(ctx, cmd)
		if err != nil {
			h.writeError(w, meta, err)
			return err
		}

		h.writeSuccess(w, meta, http.StatusOK, op)
		return nil
	})
}

func (h *DisasterRecoveryHandler) HandleDatasets(w http.ResponseWriter, r *http.Request) {
	meta := utils.ExtractIngressMetadata(r)
	_ = observability.WithSpan(r.Context(), "disaster_recovery.handler.datasets", func(ctx context.Context) error {
		path := strings.TrimPrefix(r.URL.Path, constants.EndpointDRDatasetsPrefix)

		if strings.HasSuffix(path, ":fence") && r.Method == http.MethodPost {
			datasetID := strings.TrimSuffix(path, ":fence")
			h.fenceDataset(w, r, ctx, meta, datasetID)
			return nil
		}

		if strings.HasSuffix(path, "/epoch") && r.Method == http.MethodGet {
			datasetID := strings.TrimSuffix(path, "/epoch")
			h.getDatasetEpoch(w, r, ctx, meta, datasetID)
			return nil
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
		return nil
	})
}

func (h *DisasterRecoveryHandler) HandleDrills(w http.ResponseWriter, r *http.Request) {
	meta := utils.ExtractIngressMetadata(r)
	_ = observability.WithSpan(r.Context(), "disaster_recovery.handler.drills", func(ctx context.Context) error {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return nil
		}

		var cmd contracts.RunDrillCommand
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cmd); err != nil {
			h.writeError(w, meta, appErrors.NewValidationError("invalid request body schema"))
			return err
		}

		if cmd.TenantID == "" {
			cmd.TenantID = meta.TenantID
		}

		rec, err := h.service.RunDrill(ctx, cmd)
		if err != nil {
			h.writeError(w, meta, err)
			return err
		}

		h.writeSuccess(w, meta, http.StatusOK, rec)
		return nil
	})
}

func (h *DisasterRecoveryHandler) createPlan(w http.ResponseWriter, r *http.Request, ctx context.Context, meta utils.IngressMetadata) {
	var cmd contracts.CreateDRPlanCommand
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cmd); err != nil {
		h.writeError(w, meta, appErrors.NewValidationError("invalid closed-schema request payload"))
		return
	}

	if cmd.TenantID == "" {
		cmd.TenantID = meta.TenantID
	}
	if cmd.CorrelationID == "" {
		cmd.CorrelationID = meta.CorrelationID
	}
	if cmd.IdempotencyKey == "" {
		cmd.IdempotencyKey = meta.IdempotencyKey
	}

	plan, err := h.service.CreatePlan(ctx, cmd)
	if err != nil {
		h.writeError(w, meta, err)
		return
	}

	h.writeSuccess(w, meta, http.StatusCreated, plan)
}

func (h *DisasterRecoveryHandler) listPlans(w http.ResponseWriter, r *http.Request, ctx context.Context, meta utils.IngressMetadata) {
	plans, err := h.service.ListPlans(ctx, meta.TenantID)
	if err != nil {
		h.writeError(w, meta, err)
		return
	}
	h.writeSuccess(w, meta, http.StatusOK, plans)
}

func (h *DisasterRecoveryHandler) getPlan(w http.ResponseWriter, r *http.Request, ctx context.Context, meta utils.IngressMetadata, planID string) {
	plan, err := h.service.GetPlan(ctx, meta.TenantID, planID)
	if err != nil {
		h.writeError(w, meta, err)
		return
	}
	h.writeSuccess(w, meta, http.StatusOK, plan)
}

func (h *DisasterRecoveryHandler) checkReadiness(w http.ResponseWriter, r *http.Request, ctx context.Context, meta utils.IngressMetadata, planID string) {
	result, err := h.service.CheckReadiness(ctx, meta.TenantID, planID)
	if err != nil {
		h.writeError(w, meta, err)
		return
	}
	h.writeSuccess(w, meta, http.StatusOK, result)
}

func (h *DisasterRecoveryHandler) approveFailover(w http.ResponseWriter, r *http.Request, ctx context.Context, meta utils.IngressMetadata, opID string) {
	var cmd contracts.ApproveFailoverCommand
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cmd); err != nil {
		h.writeError(w, meta, appErrors.NewValidationError("invalid request body schema"))
		return
	}

	cmd.OperationID = opID
	if cmd.TenantID == "" {
		cmd.TenantID = meta.TenantID
	}

	op, err := h.service.ApproveAndExecuteFailover(ctx, cmd)
	if err != nil {
		h.writeError(w, meta, err)
		return
	}

	h.writeSuccess(w, meta, http.StatusOK, op)
}

func (h *DisasterRecoveryHandler) abortFailover(w http.ResponseWriter, r *http.Request, ctx context.Context, meta utils.IngressMetadata, opID string) {
	var cmd contracts.AbortFailoverCommand
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cmd); err != nil {
		h.writeError(w, meta, appErrors.NewValidationError("invalid request body schema"))
		return
	}

	cmd.OperationID = opID
	if cmd.TenantID == "" {
		cmd.TenantID = meta.TenantID
	}

	op, err := h.service.AbortFailover(ctx, cmd)
	if err != nil {
		h.writeError(w, meta, err)
		return
	}

	h.writeSuccess(w, meta, http.StatusOK, op)
}

func (h *DisasterRecoveryHandler) fenceDataset(w http.ResponseWriter, r *http.Request, ctx context.Context, meta utils.IngressMetadata, datasetID string) {
	var cmd contracts.FenceDatasetCommand
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cmd); err != nil {
		h.writeError(w, meta, appErrors.NewValidationError("invalid request body schema"))
		return
	}

	cmd.DatasetID = datasetID
	if cmd.TenantID == "" {
		cmd.TenantID = meta.TenantID
	}

	epoch, err := h.service.FenceDataset(ctx, cmd)
	if err != nil {
		h.writeError(w, meta, err)
		return
	}

	h.writeSuccess(w, meta, http.StatusOK, epoch)
}

func (h *DisasterRecoveryHandler) getDatasetEpoch(w http.ResponseWriter, r *http.Request, ctx context.Context, meta utils.IngressMetadata, datasetID string) {
	epoch, err := h.service.GetFencingEpoch(ctx, datasetID)
	if err != nil {
		h.writeError(w, meta, err)
		return
	}
	h.writeSuccess(w, meta, http.StatusOK, epoch)
}

func (h *DisasterRecoveryHandler) writeSuccess(w http.ResponseWriter, meta utils.IngressMetadata, statusCode int, data interface{}) {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	if meta.Traceparent != "" {
		w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)
	}
	if meta.RequestID != "" {
		w.Header().Set(constants.HeaderXRequestID, meta.RequestID)
	}
	w.WriteHeader(statusCode)

	resp := types.ApiResponse{
		Success:    true,
		StatusCode: statusCode,
		Data:       data,
		Meta:       utils.BuildResponseMeta(meta),
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (h *DisasterRecoveryHandler) writeError(w http.ResponseWriter, meta utils.IngressMetadata, err error) {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	if meta.Traceparent != "" {
		w.Header().Set(constants.HeaderTraceparent, meta.Traceparent)
	}
	if meta.RequestID != "" {
		w.Header().Set(constants.HeaderXRequestID, meta.RequestID)
	}

	statusCode := http.StatusInternalServerError
	var appErr *appErrors.ApplicationError
	if errorsAs(err, &appErr) {
		statusCode = appErr.StatusCode
	}

	w.WriteHeader(statusCode)
	resp := types.ApiResponse{
		Success:    false,
		StatusCode: statusCode,
		Error: &types.ResponseError{
			Code:    "DISASTER_RECOVERY_ERROR",
			Message: err.Error(),
		},
		Meta: utils.BuildResponseMeta(meta),
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func errorsAs(err error, target **appErrors.ApplicationError) bool {
	if ae, ok := err.(*appErrors.ApplicationError); ok {
		*target = ae
		return true
	}
	return false
}
