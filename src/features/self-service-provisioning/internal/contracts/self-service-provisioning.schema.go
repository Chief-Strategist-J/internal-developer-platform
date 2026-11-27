/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING SCHEMA ACL (V.3 role 03)

 1. Role:
    - Strict anti-corruption decoding (DisallowUnknownFields) and envelope serialization.
 2. Invariants:
    - Zero inline comments inside function bodies.
*/
package contracts

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/types"
)

func DecodeClusterSpec(body io.Reader) (ClusterSpec, error) {
	var spec ClusterSpec
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&spec)
	return spec, err
}

func DecodeWorkloadIdentitySpec(body io.Reader) (WorkloadIdentitySpec, error) {
	var spec WorkloadIdentitySpec
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&spec)
	return spec, err
}

func DecodeReconcileLedgerRequest(body io.Reader) (ReconcileLedgerRequest, error) {
	var req ReconcileLedgerRequest
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	err := dec.Decode(&req)
	return req, err
}

func WriteErrorEnvelope(w http.ResponseWriter, status int, code string, msg string) {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(types.ApiResponse{
		Success:    false,
		StatusCode: status,
		Error:      &types.ResponseError{Code: code, Message: msg},
	})
}

func WriteSuccessEnvelope(w http.ResponseWriter, status int, traceparent string, reqID string, corrID string, startedAt time.Time, data interface{}) error {
	w.Header().Set(constants.HeaderContentType, constants.ContentTypeJSON)
	if traceparent != "" {
		w.Header().Set(constants.HeaderTraceparent, traceparent)
	}
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(types.ApiResponse{
		Success:    true,
		StatusCode: status,
		Data:       data,
		Meta: types.ResponseMeta{
			RequestID:       reqID,
			CorrelationID:   corrID,
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
			ExecutionTimeMs: time.Since(startedAt).Milliseconds(),
			APIVersion:      "v1",
		},
	})
}
