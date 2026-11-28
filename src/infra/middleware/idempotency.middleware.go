/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DURABLE HTTP IDEMPOTENCY MIDDLEWARE (V.3 Part VI.1 M-01, EC-02)

 1. Role (Layer 1, G-05):
    - Enforces Idempotency-Key / X-Idempotency-Key deduplication for unsafe HTTP methods.
    - Persists keys, locks, and responses in durable Redis storage with automatic memory fallback.
    - Replays stored responses for identical (principal, tenant, method, path, key, payloadHash).
    - Rejects mismatched payloads for an existing key with 422 Unprocessable Entity and concurrent in-flight duplicates with 409 Conflict.
 2. Invariants:
    - Distributed Redis locks with 30s TTL to prevent deadlock on handler crash.
    - Distributed response caching with 24h TTL for deterministic safe replay.
    - Zero inline comments inside function bodies below this header.
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
	"time"

	"github.com/redis/go-redis/v9"

	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/types"
)

const (
	defaultLockTTL      = 30 * time.Second
	defaultRetentionTTL = 24 * time.Hour
)

type idempotencyRecord struct {
	InFlight    bool   `json:"inFlight"`
	BodyHash    string `json:"bodyHash"`
	StatusCode  int    `json:"statusCode"`
	ContentType string `json:"contentType"`
	Body        []byte `json:"body"`
}

type IdempotencyMiddleware struct {
	mu          sync.Mutex
	records     map[string]idempotencyRecord
	redisClient *redis.Client
}

func NewIdempotencyMiddleware() *IdempotencyMiddleware {
	return &IdempotencyMiddleware{
		records: make(map[string]idempotencyRecord),
	}
}

func NewIdempotencyMiddlewareWithAddr(addr string) *IdempotencyMiddleware {
	m := NewIdempotencyMiddleware()
	if addr != "" {
		m.redisClient = redis.NewClient(&redis.Options{
			Addr:         addr,
			DialTimeout:  2 * time.Second,
			ReadTimeout:  2 * time.Second,
			WriteTimeout: 2 * time.Second,
		})
	}
	return m
}

func NewIdempotencyMiddlewareWithRedis(client *redis.Client) *IdempotencyMiddleware {
	return &IdempotencyMiddleware{
		records:     make(map[string]idempotencyRecord),
		redisClient: client,
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

		if m.redisClient != nil {
			ctx := r.Context()
			recJSON, getErr := m.redisClient.Get(ctx, "idemp:"+compositeKey).Bytes()
			if getErr == nil {
				var rec idempotencyRecord
				if decErr := json.Unmarshal(recJSON, &rec); decErr == nil {
					if rec.InFlight {
						writeIdempotencyError(w, http.StatusConflict, constants.ErrCodeConflict, "idempotent request is currently in flight")
						return
					}
					if rec.BodyHash != bodyHash {
						writeIdempotencyError(w, http.StatusUnprocessableEntity, constants.ErrCodeValidationFailed, "idempotency key reused with different payload")
						return
					}
					if rec.ContentType != "" {
						w.Header().Set("Content-Type", rec.ContentType)
					}
					w.WriteHeader(rec.StatusCode)
					_, _ = w.Write(rec.Body)
					return
				}
			}

			inFlightRecord := idempotencyRecord{InFlight: true, BodyHash: bodyHash}
			inFlightBytes, _ := json.Marshal(inFlightRecord)
			acquired, setErr := m.redisClient.SetNX(ctx, "idemp:"+compositeKey, inFlightBytes, defaultLockTTL).Result()
			if setErr == nil && !acquired {
				writeIdempotencyError(w, http.StatusConflict, constants.ErrCodeConflict, "idempotent request is currently in flight")
				return
			}

			capWriter := &responseCaptureWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next(capWriter, r)

			if capWriter.statusCode >= http.StatusInternalServerError {
				_ = m.redisClient.Del(ctx, "idemp:"+compositeKey).Err()
				return
			}

			finalRecord := idempotencyRecord{
				InFlight:    false,
				BodyHash:    bodyHash,
				StatusCode:  capWriter.statusCode,
				ContentType: capWriter.Header().Get("Content-Type"),
				Body:        capWriter.buf.Bytes(),
			}
			finalBytes, _ := json.Marshal(finalRecord)
			_ = m.redisClient.Set(ctx, "idemp:"+compositeKey, finalBytes, defaultRetentionTTL).Err()
			return
		}

		m.mu.Lock()
		existing, exists := m.records[compositeKey]
		if exists {
			m.mu.Unlock()
			if existing.InFlight {
				writeIdempotencyError(w, http.StatusConflict, constants.ErrCodeConflict, "idempotent request is currently in flight")
				return
			}
			if existing.BodyHash != bodyHash {
				writeIdempotencyError(w, http.StatusUnprocessableEntity, constants.ErrCodeValidationFailed, "idempotency key reused with different payload")
				return
			}
			if existing.ContentType != "" {
				w.Header().Set("Content-Type", existing.ContentType)
			}
			w.WriteHeader(existing.StatusCode)
			_, _ = w.Write(existing.Body)
			return
		}
		m.records[compositeKey] = idempotencyRecord{InFlight: true, BodyHash: bodyHash}
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
			InFlight:    false,
			BodyHash:    bodyHash,
			StatusCode:  capWriter.statusCode,
			ContentType: capWriter.Header().Get("Content-Type"),
			Body:        capWriter.buf.Bytes(),
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
