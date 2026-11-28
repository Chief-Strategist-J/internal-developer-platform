/*
ALGORITHM & ARCHITECTURE BLUEPRINT: IDEMPOTENCY ERROR META TEST

1. Purpose:
   - Idempotency middleware rejections use the canonical envelope with populated meta and echoed headers.
2. Invariants:
   - Zero inline comments below this header.
*/

package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"internal-developer-platform/src/shared/types"
)

func TestWriteIdempotencyError_PopulatesMetaAndEchoHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.Header.Set("x-request-id", "req-9")
	req.Header.Set("x-correlation-id", "corr-9")
	rec := httptest.NewRecorder()
	writeIdempotencyError(rec, req, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "idempotency key reused with different payload")

	var env types.ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Meta.RequestID != "req-9" || env.Meta.CorrelationID != "corr-9" || env.Meta.Timestamp == "" || env.Meta.APIVersion == "" {
		t.Fatalf("meta not populated: %+v", env.Meta)
	}
	if rec.Header().Get("x-request-id") != "req-9" {
		t.Fatalf("x-request-id not echoed: %v", rec.Header())
	}
}
