// Package handlers provides HTTP REST handlers for Cloud Resource Provisioning.
//
// Architectural Blueprint:
// - Step 02 Handler: Exposes Cloud Resource API endpoints (§4.2 Workload Identity, Buckets, DNS).
// - Enforces OpenTelemetry child spans and strict envelope responses.
// - 100% comment-free function bodies.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"internal-developer-platform/src/infra/clients/cloudresource"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/types"
)

type CloudResourceHandler struct {
	client *cloudresource.CloudResourceClient
}

func NewCloudResourceHandler(client *cloudresource.CloudResourceClient) *CloudResourceHandler {
	return &CloudResourceHandler{client: client}
}

func (h *CloudResourceHandler) ProvisionResource(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "cloudresource.handler.provision", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)

		var spec cloudresource.WorkloadIdentityBindingSpec
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

		res, err := h.client.ProvisionWorkloadIdentity(ctx, spec)
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
			Data:       res,
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
