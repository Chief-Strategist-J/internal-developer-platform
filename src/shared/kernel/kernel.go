// Package kernel declares the shared kernel interfaces and pure types
// for the internal-developer-platform.
//
// Blueprint:
//  1. All types here are interface-only or pure value types — no IO, no state, no business logic.
//  2. Features instantiate these generics; they MUST NOT redefine them locally (K-01).
//  3. A new type needs at least two real consumers before promotion to this package.
//
// Invariants:
//   - No import of any infrastructure library (database drivers, HTTP, Redis, etc.).
//   - No feature-specific types. Only types shared by 2+ features belong here.
package kernel

import (
	"context"
	"time"
)

type PortErrorKind string

const (
	PortErrorNotFound    PortErrorKind = "not_found"
	PortErrorConflict    PortErrorKind = "conflict"
	PortErrorUnavailable PortErrorKind = "unavailable"
	PortErrorRejected    PortErrorKind = "rejected"
	PortErrorInternal    PortErrorKind = "internal"
)

type PortError struct {
	Kind      PortErrorKind
	Message   string
	Cause     error
	Retryable bool
}

func (e *PortError) Error() string {
	if e.Cause != nil {
		return string(e.Kind) + ": " + e.Message + ": " + e.Cause.Error()
	}
	return string(e.Kind) + ": " + e.Message
}

func (e *PortError) Unwrap() error { return e.Cause }

type Envelope[T any] struct {
	Success    bool   `json:"success"`
	StatusCode int    `json:"statusCode"`
	Data       *T     `json:"data,omitempty"`
	Error      *Error `json:"error,omitempty"`
	Meta       Meta   `json:"meta"`
}

type Error struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

type Meta struct {
	TraceID string `json:"traceId"`
}

type Principal struct {
	Subject  string
	TenantID string
	Roles    []string
	Scopes   []string
	AuthTime time.Time
}

type EventEnvelope[T any] struct {
	EventID          string    `json:"eventId"`
	Type             string    `json:"type"`
	SchemaVersion    string    `json:"schemaVersion"`
	AggregateID      string    `json:"aggregateId"`
	AggregateVersion int64     `json:"aggregateVersion"`
	OccurredAt       time.Time `json:"occurredAt"`
	TenantID         string    `json:"tenantId"`
	Principal        string    `json:"principal"`
	Traceparent      string    `json:"traceparent"`
	Payload          T         `json:"payload"`
}

type Lifecycle interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Ready() bool
}

type Module interface {
	Name() string
	Routes() []Route
	EventHandlers() []EventHandler
	Lifecycles() []Lifecycle
	Provides() []string
}

type Route struct {
	Method  string
	Pattern string
	Handler any
}

type EventHandler struct {
	EventType string
	Handler   any
}
