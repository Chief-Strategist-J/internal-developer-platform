// Package utils provides centralized pure utility functions for ingress metadata and response formatting.
//
// Architectural Blueprint:
// - Step 15 Utils: Standardized helpers for extracting W3C ingress headers and constructing response meta.
// - Eliminates repetitive boilerplate across feature handlers per 3.shared.code.writing.rule.md Guardrail 5.
// - Zero business logic, zero database calls, zero I/O.
// - 100% comment-free function bodies.
package utils

import (
	"net/http"
	"time"

	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/types"
)

type IngressMetadata struct {
	Traceparent    string
	RequestID      string
	CorrelationID  string
	CausationID    string
	TenantID       string
	ClientID       string
	UserID         string
	IdempotencyKey string
	APIVersion     string
	IfMatch        string
	StartTime      time.Time
}

func ExtractIngressMetadata(r *http.Request) IngressMetadata {
	return IngressMetadata{
		Traceparent:    r.Header.Get(constants.HeaderTraceparent),
		RequestID:      r.Header.Get(constants.HeaderXRequestID),
		CorrelationID:  r.Header.Get(constants.HeaderXCorrelationID),
		CausationID:    r.Header.Get(constants.HeaderXCausationID),
		TenantID:       r.Header.Get(constants.HeaderXTenantID),
		ClientID:       r.Header.Get(constants.HeaderXClientID),
		UserID:         r.Header.Get(constants.HeaderXUserID),
		IdempotencyKey: r.Header.Get(constants.HeaderXIdempotencyKey),
		APIVersion:     r.Header.Get(constants.HeaderXAPIVersion),
		IfMatch:        r.Header.Get(constants.HeaderIfMatch),
		StartTime:      time.Now(),
	}
}

func BuildResponseMeta(meta IngressMetadata) types.ResponseMeta {
	apiVer := meta.APIVersion
	if apiVer == "" {
		apiVer = "v1"
	}
	return types.ResponseMeta{
		RequestID:       meta.RequestID,
		CorrelationID:   meta.CorrelationID,
		CausationID:     meta.CausationID,
		Timestamp:       time.Now().UTC().Format(time.RFC3339Nano),
		ExecutionTimeMs: time.Since(meta.StartTime).Milliseconds(),
		APIVersion:      apiVer,
	}
}

