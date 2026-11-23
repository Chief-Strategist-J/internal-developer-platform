// Package handlers provides HTTP REST handlers for GKE Cluster Provisioning.
//
// Architectural Blueprint:
// - Step 02 Handler: Exposes Cluster API endpoints (§4.1 Hardened GKE baseline).
// - Enforces OpenTelemetry child spans and strict envelope responses.
// - 100% comment-free function bodies.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"internal-developer-platform/src/infra/clients/cluster"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/types"
)

type ClusterHandler struct {
	client *cluster.GKEClusterClient
}

func NewClusterHandler(client *cluster.GKEClusterClient) *ClusterHandler {
	return &ClusterHandler{client: client}
}

func (h *ClusterHandler) ProvisionCluster(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "cluster.handler.provision", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)

		var spec cluster.GKEClusterSpec
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&spec); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(types.ApiResponse{
				Success:    false,
				StatusCode: http.StatusBadRequest,
				Error:      &types.ResponseError{Code: constants.ErrCodeValidationFailed, Message: err.Error()},
			})
			return err
		}

		rec, err := h.client.ProvisionCluster(ctx, spec)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(types.ApiResponse{
				Success:    false,
				StatusCode: http.StatusBadRequest,
				Error:      &types.ResponseError{Code: constants.ErrCodeValidationFailed, Message: err.Error()},
			})
			return err
		}

		w.WriteHeader(http.StatusCreated)
		w.Header().Set(constants.HeaderTraceparent, traceparent)
		return json.NewEncoder(w).Encode(types.ApiResponse{
			Success:    true,
			StatusCode: http.StatusCreated,
			Data:       rec,
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

func (h *ClusterHandler) GetCluster(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "cluster.handler.get", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		clusterID := r.URL.Query().Get("clusterId")

		rec, err := h.client.GetCluster(ctx, clusterID)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(types.ApiResponse{
				Success:    false,
				StatusCode: http.StatusNotFound,
				Error:      &types.ResponseError{Code: constants.ErrCodeNotFound, Message: err.Error()},
			})
			return err
		}

		w.WriteHeader(http.StatusOK)
		return json.NewEncoder(w).Encode(types.ApiResponse{
			Success:    true,
			StatusCode: http.StatusOK,
			Data:       rec,
			Meta: types.ResponseMeta{
				Timestamp:       time.Now().UTC().Format(time.RFC3339),
				ExecutionTimeMs: time.Since(start).Milliseconds(),
				APIVersion:      "v1",
			},
		})
	})
}
