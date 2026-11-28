/*
ALGORITHM & ARCHITECTURE BLUEPRINT: AUTOSCALING REST TEST SUITE (V.3 role 14)

1. Purpose:
   - Verifies the canonical envelope and meta, header echo, tenant-from-header, closed-schema rejection,
     validation mapping, and method handling.
2. Invariants:
   - Zero inline comments below this header.
*/

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	restv1 "internal-developer-platform/src/features/autoscaling/internal/api/rest/v1"
	"internal-developer-platform/src/features/autoscaling/internal/service"
	"internal-developer-platform/src/shared/types"
)

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	handler := restv1.NewAutoscalingHandler(service.NewAutoscalingService())
	restv1.NewAutoscalingRouter(handler, nil, nil).RegisterRoutes(mux)
	return mux
}

func post(t *testing.T, body []byte, tenant string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/autoscaling/plans:evaluate", bytes.NewReader(body))
	req.Header.Set("x-tenant-id", tenant)
	req.Header.Set("x-request-id", "req-1")
	req.Header.Set("x-correlation-id", "corr-1")
	rec := httptest.NewRecorder()
	newMux().ServeHTTP(rec, req)
	return rec
}

func TestRest_EvaluateCompliantPlan(t *testing.T) {
	body, err := json.Marshal(compliantInput())
	if err != nil {
		t.Fatal(err)
	}
	rec := post(t, body, "tenant-a")
	var env types.ApiResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &env) != nil || !env.Success {
		t.Fatalf("unexpected response %d: %s", rec.Code, rec.Body.String())
	}
	if env.Meta.RequestID != "req-1" || env.Meta.APIVersion == "" || env.Meta.Timestamp == "" {
		t.Fatalf("meta not populated: %+v", env.Meta)
	}
	if rec.Header().Get("x-request-id") != "req-1" || rec.Header().Get("x-correlation-id") != "corr-1" {
		t.Fatalf("echo headers missing: %v", rec.Header())
	}
}

func TestRest_TenantComesFromHeaderOnly(t *testing.T) {
	body, err := json.Marshal(compliantInput())
	if err != nil {
		t.Fatal(err)
	}
	if rec := post(t, body, ""); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 without tenant header, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRest_Int64QuantitiesAreStrings(t *testing.T) {
	body, err := json.Marshal(compliantInput())
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	if got := string(raw["ceilings"]["quotaPods"]); got != `"20"` {
		t.Fatalf("expected string-serialized int64, got %s", got)
	}
}

func TestRest_RejectsUnknownFields(t *testing.T) {
	if rec := post(t, []byte(`{"plan":{},"unexpected":true}`), "tenant-a"); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestRest_ValidationFailureMapsTo422(t *testing.T) {
	rec := post(t, []byte(`{}`), "tenant-a")
	var env types.ApiResponse
	if rec.Code != http.StatusUnprocessableEntity || json.Unmarshal(rec.Body.Bytes(), &env) != nil || env.Error == nil {
		t.Fatalf("expected 422 with error object, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRest_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/autoscaling/plans:evaluate", nil)
	rec := httptest.NewRecorder()
	newMux().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
