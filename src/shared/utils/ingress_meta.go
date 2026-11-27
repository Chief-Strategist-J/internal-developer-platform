// Package utils provides centralized pure utility functions for ingress metadata and response formatting.
//
// Architectural Blueprint:
// - Step 15 Utils: Pure formatting helpers without net/http IO imports (G-05).
// - Zero business logic, zero database calls, zero I/O.
// - 100% comment-free function bodies.
package utils

import (
	"time"

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
