package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	healthService "internal-developer-platform/src/features/health-checks/service"
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
	w.Header().Set("Content-Type", "application/json")
	if h.service.CheckLiveness(r.Context()) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"alive"}`))
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte(`{"status":"dead"}`))
}

func (h *HealthCheckHandler) HandleReadiness(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.service.CheckReadiness(r.Context()) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ready"}`))
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	w.Write([]byte(`{"status":"unready"}`))
}

func (h *HealthCheckHandler) HandleSystemHealth(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	w.Header().Set("Content-Type", "application/json")
	traceparent := r.Header.Get(constants.HeaderTraceparent)
	reqID := r.Header.Get(constants.HeaderXRequestID)
	corrID := r.Header.Get(constants.HeaderXCorrelationID)

	report := h.service.EvaluateSystemHealth(r.Context())

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
	json.NewEncoder(w).Encode(resp)
}
