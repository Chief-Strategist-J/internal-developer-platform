/*
ALGORITHM & ARCHITECTURE BLUEPRINT: HTTP IDEMPOTENCY MIDDLEWARE (V.3 Part VI.1 M-01, EC-02)

 1. Role (Layer 1, G-05):
    - Enforces Idempotency-Key / X-Idempotency-Key deduplication for unsafe HTTP methods.
    - Replays stored responses for identical (principal, tenant, method, path, key, payloadHash).
    - Rejects mismatched payloads for an existing key with 422 Unprocessable Entity and concurrent in-flight duplicates with 409 Conflict.
 2. Invariants:
    - Concurrency-safe in-memory store safe to lose on restart (F-02).
    - Zero inline comments below this header.
*/
package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sync"

	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/types"
)

type idempotencyRecord struct {
	inFlight    bool
	bodyHash    string
	statusCode  int
	contentType string
	body        []byte
}

type IdempotencyMiddleware struct {
	mu      sync.Mutex
	records map[string]idempotencyRecord
}

func NewIdempotencyMiddleware() *IdempotencyMiddleware {
	return &IdempotencyMiddleware{
		records: make(map[string]idempotencyRecord),
	}
}

func (m *IdempotencyMiddleware) Wrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isUnsafeMethod(r.Method) {
			next(w, r)
			return
		}
		key := r.Header.Get(constants.HeaderXIdempotencyKey)
		if key == "" {
			key = r.Header.Get("Idempotency-Key")
		}
		if key == "" {
			next(w, r)
			return
		}

		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			writeIdempotencyError(w, http.StatusBadRequest, constants.ErrCodeValidationFailed, "failed to read request body")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		sum := sha256.Sum256(bodyBytes)
		bodyHash := hex.EncodeToString(sum[:])

		compositeKey := buildIdempotencyKey(r, key)

		m.mu.Lock()
		existing, exists := m.records[compositeKey]
		if exists {
			m.mu.Unlock()
			if existing.inFlight {
				writeIdempotencyError(w, http.StatusConflict, constants.ErrCodeConflict, "idempotent request is currently in flight")
				return
			}
			if existing.bodyHash != bodyHash {
				writeIdempotencyError(w, http.StatusUnprocessableEntity, constants.ErrCodeValidationFailed, "idempotency key reused with different payload")
				return
			}
			if existing.contentType != "" {
				w.Header().Set("Content-Type", existing.contentType)
			}
			w.WriteHeader(existing.statusCode)
			_, _ = w.Write(existing.body)
			return
		}
		m.records[compositeKey] = idempotencyRecord{inFlight: true, bodyHash: bodyHash}
		m.mu.Unlock()

		capWriter := &responseCaptureWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next(capWriter, r)

		m.mu.Lock()
		defer m.mu.Unlock()
		if capWriter.statusCode >= http.StatusInternalServerError {
			delete(m.records, compositeKey)
			return
		}
		m.records[compositeKey] = idempotencyRecord{
			inFlight:    false,
			bodyHash:    bodyHash,
			statusCode:  capWriter.statusCode,
			contentType: capWriter.Header().Get("Content-Type"),
			body:        capWriter.buf.Bytes(),
		}
	}
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func buildIdempotencyKey(r *http.Request, key string) string {
	principal := r.Header.Get(constants.HeaderXUserID)
	tenant := r.Header.Get(constants.HeaderXTenantID)
	return principal + "|" + tenant + "|" + r.Method + "|" + r.URL.Path + "|" + key
}

func writeIdempotencyError(w http.ResponseWriter, status int, code string, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(types.ApiResponse{
		Success:    false,
		StatusCode: status,
		Error: &types.ResponseError{
			Code:    code,
			Message: msg,
		},
	})
}

type responseCaptureWriter struct {
	http.ResponseWriter
	statusCode int
	buf        bytes.Buffer
}

func (w *responseCaptureWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseCaptureWriter) Write(p []byte) (int, error) {
	_, _ = w.buf.Write(p)
	return w.ResponseWriter.Write(p)
}
