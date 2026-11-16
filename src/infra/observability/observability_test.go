package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStructuredLoggingFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf, "test-platform-service")
	ctx := WithTenantID(context.Background(), "tenant-alpha")

	logger.Info(ctx, "workload provisioned successfully", String("workload_id", "wk-1234"), Int("replicas", 3))

	var parsed CanonicalLogEntry
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse log json output: %v", err)
	}

	if parsed.Service != "test-platform-service" {
		t.Errorf("expected service 'test-platform-service', got '%s'", parsed.Service)
	}
	if parsed.Level != "INFO" {
		t.Errorf("expected level 'INFO', got '%s'", parsed.Level)
	}
	if parsed.Message != "workload provisioned successfully" {
		t.Errorf("unexpected message: '%s'", parsed.Message)
	}
	if parsed.TenantID != "tenant-alpha" {
		t.Errorf("expected tenant_id 'tenant-alpha', got '%s'", parsed.TenantID)
	}
	if parsed.Attributes["workload_id"] != "wk-1234" {
		t.Errorf("expected workload_id 'wk-1234', got '%v'", parsed.Attributes["workload_id"])
	}
}

func TestTelemetryMiddlewareTraceContextPropagation(t *testing.T) {
	_, shutdown, err := InitTracer("test-service")
	if err != nil {
		t.Fatalf("failed to init tracer: %v", err)
	}
	defer func() {
		_ = shutdown(context.Background())
	}()

	handler := IngressTelemetryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := ExtractTenantID(r.Context())
		if tenant != "tenant-acme" {
			t.Errorf("expected tenant context 'tenant-acme', got '%s'", tenant)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads", nil)
	req.Header.Set("X-Tenant-Id", "tenant-acme")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	traceID := rec.Header().Get("X-Trace-Id")
	spanID := rec.Header().Get("X-Span-Id")

	if traceID == "" {
		t.Error("expected non-empty X-Trace-Id header")
	}
	if spanID == "" {
		t.Error("expected non-empty X-Span-Id header")
	}
}

func TestTelemetryMiddlewareClientErrorStatus(t *testing.T) {
	handler := IngressTelemetryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad_request"}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workloads", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}
