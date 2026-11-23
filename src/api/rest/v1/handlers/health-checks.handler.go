// Package handlers provides HTTP REST handlers for health check probes.
//
// Architectural Blueprint:
// - Step 02 Handler: Exposes liveness, readiness, and system health endpoints.
// - Spawns Tier-2 handler child spans via observability.WithSpan.
// - 100% comment-free function bodies.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	healthService "internal-developer-platform/src/features/health-checks/service"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/types"
)

type HealthCheckHandler struct {
	service *healthService.HealthCheckService
}

func NewHealthCheckHandler(s *healthService.HealthCheckService) *HealthCheckHandler {
	return &HealthCheckHandler{service: s}
}

func (h *HealthCheckHandler) HandleLiveness(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "health.handler.liveness", func(ctx context.Context) error {
		w.Header().Set("Content-Type", "application/json")
		if h.service.CheckLiveness(ctx) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"alive"}`))
			return nil
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"dead"}`))
		return nil
	})
}

func (h *HealthCheckHandler) HandleReadiness(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "health.handler.readiness", func(ctx context.Context) error {
		w.Header().Set("Content-Type", "application/json")
		if h.service.CheckReadiness(ctx) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ready"}`))
			return nil
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"unready"}`))
		return nil
	})
}

func (h *HealthCheckHandler) HandleSystemHealth(w http.ResponseWriter, r *http.Request) {
	_ = observability.WithSpan(r.Context(), "health.handler.system_health", func(ctx context.Context) error {
		start := time.Now()
		w.Header().Set("Content-Type", "application/json")
		traceparent := r.Header.Get(constants.HeaderTraceparent)
		reqID := r.Header.Get(constants.HeaderXRequestID)
		corrID := r.Header.Get(constants.HeaderXCorrelationID)

		report := h.service.EvaluateSystemHealth(ctx)

		statusCode := http.StatusOK
		if report.Status == "UNHEALTHY" {
			statusCode = http.StatusServiceUnavailable
		}

		w.WriteHeader(statusCode)
		w.Header().Set(constants.HeaderTraceparent, traceparent)
		resp := types.ApiResponse{
			Success:    statusCode == http.StatusOK,
			StatusCode: statusCode,
			Data:       report,
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
