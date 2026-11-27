/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING REST HANDLER (V.3 role 02)

 1. Role:
    - Exposes GKE Cluster and Cloud Resource HTTP endpoints inside self-service-provisioning.
 2. Invariants:
    - Opens OTel child spans and enforces strict ACL decoding + envelope responses.
    - Zero inline comments below this header.
*/
package restv1

import (
	"context"
	"net/http"
	"time"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/features/self-service-provisioning/internal/service"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
)

type SelfServiceProvisioningHandler struct {
	svc *service.SelfServiceProvisioningService
}

func NewSelfServiceProvisioningHandler(svc *service.SelfServiceProvisioningService) *SelfServiceProvisioningHandler {
	return &SelfServiceProvisioningHandler{svc: svc}
}

func (h *SelfServiceProvisioningHandler) ProvisionCluster(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "self_service.cluster.provision", func(ctx context.Context) error {
		start := time.Now()
		meta := middleware.ExtractIngressMetadata(r)
		spec, err := contracts.DecodeClusterSpec(r.Body)
		if err != nil {
			contracts.WriteErrorEnvelope(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error())
			return err
		}
		rec, err := h.svc.ProvisionCluster(ctx, spec)
		if err != nil {
			contracts.WriteErrorEnvelope(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error())
			return err
		}
		return contracts.WriteSuccessEnvelope(w, http.StatusCreated, meta.Traceparent, meta.RequestID, meta.CorrelationID, start, rec)
	})
}

func (h *SelfServiceProvisioningHandler) GetCluster(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "self_service.cluster.get", func(ctx context.Context) error {
		start := time.Now()
		meta := middleware.ExtractIngressMetadata(r)
		clusterID := r.URL.Query().Get("clusterId")
		rec, err := h.svc.GetCluster(ctx, clusterID)
		if err != nil {
			contracts.WriteErrorEnvelope(w, http.StatusNotFound, constants.ErrCodeNotFound, err.Error())
			return err
		}
		return contracts.WriteSuccessEnvelope(w, http.StatusOK, meta.Traceparent, meta.RequestID, meta.CorrelationID, start, rec)
	})
}

func (h *SelfServiceProvisioningHandler) ValidatePosture(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "self_service.cluster.validate_posture", func(ctx context.Context) error {
		start := time.Now()
		meta := middleware.ExtractIngressMetadata(r)
		clusterID := r.URL.Query().Get("clusterId")
		report, err := h.svc.ValidateClusterPosture(ctx, clusterID)
		if err != nil {
			contracts.WriteErrorEnvelope(w, http.StatusNotFound, constants.ErrCodeNotFound, err.Error())
			return err
		}
		return contracts.WriteSuccessEnvelope(w, http.StatusOK, meta.Traceparent, meta.RequestID, meta.CorrelationID, start, report)
	})
}

func (h *SelfServiceProvisioningHandler) ProvisionResource(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "self_service.cloudresource.provision", func(ctx context.Context) error {
		start := time.Now()
		meta := middleware.ExtractIngressMetadata(r)
		spec, err := contracts.DecodeWorkloadIdentitySpec(r.Body)
		if err != nil {
			contracts.WriteErrorEnvelope(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error())
			return err
		}
		res, err := h.svc.ProvisionWorkloadIdentity(ctx, spec)
		if err != nil {
			contracts.WriteErrorEnvelope(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error())
			return err
		}
		return contracts.WriteSuccessEnvelope(w, http.StatusCreated, meta.Traceparent, meta.RequestID, meta.CorrelationID, start, res)
	})
}

func (h *SelfServiceProvisioningHandler) ReconcileLedger(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "self_service.cloudresource.reconcile", func(ctx context.Context) error {
		start := time.Now()
		meta := middleware.ExtractIngressMetadata(r)
		req, err := contracts.DecodeReconcileLedgerRequest(r.Body)
		if err != nil {
			contracts.WriteErrorEnvelope(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, err.Error())
			return err
		}
		report, err := h.svc.ReconcileLedger(ctx, req.ActiveWorkloadIDs)
		if err != nil {
			contracts.WriteErrorEnvelope(w, http.StatusInternalServerError, constants.ErrCodeInternalError, err.Error())
			return err
		}
		return contracts.WriteSuccessEnvelope(w, http.StatusOK, meta.Traceparent, meta.RequestID, meta.CorrelationID, start, report)
	})
}
