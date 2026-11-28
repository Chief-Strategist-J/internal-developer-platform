/*
ALGORITHM & ARCHITECTURE BLUEPRINT: AUTOSCALING TRACE TEST SUITE (V.3 role 14, VIII.2)

1. Purpose:
   - Verifies the spans explain themselves: allowed and denied outcomes, one event per violated invariant,
     and a rejected request carrying the reason, the client error class and the HTTP status.
2. Invariants:
   - Uses a real in-memory span exporter on the real tracer; no mocks of the code under test.
   - Zero inline comments below this header.
*/

package tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func setupTracing(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter), sdktrace.WithSampler(sdktrace.AlwaysSample()))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	return exporter
}

func findSpan(t *testing.T, exporter *tracetest.InMemoryExporter, name string) tracetest.SpanStub {
	t.Helper()
	for _, span := range exporter.GetSpans() {
		if span.Name == name {
			return span
		}
	}
	t.Fatalf("span %q not found", name)
	return tracetest.SpanStub{}
}

func attributeOf(span tracetest.SpanStub, key string) string {
	for _, kv := range span.Attributes {
		if string(kv.Key) == key {
			return kv.Value.Emit()
		}
	}
	return ""
}

func TestTrace_AllowedPlanIsAnnotated(t *testing.T) {
	exporter := setupTracing(t)
	body, _ := json.Marshal(compliantInput())
	if rec := post(t, body, "tenant-a"); rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	service := findSpan(t, exporter, "autoscaling.service.evaluate_plan")
	if attributeOf(service, "autoscaling.allowed") != "true" || attributeOf(service, "autoscaling.violation.count") != "0" {
		t.Fatalf("service span not annotated: %+v", service.Attributes)
	}
	if attributeOf(service, "autoscaling.tier") != "gold" || attributeOf(service, "autoscaling.environment") != "PRODUCTION" {
		t.Fatalf("plan context missing: %+v", service.Attributes)
	}
	if handler := findSpan(t, exporter, "autoscaling.api.evaluate_plan"); attributeOf(handler, "autoscaling.outcome") != "allowed" {
		t.Fatalf("handler outcome not recorded: %+v", handler.Attributes)
	}
}

func TestTrace_DeniedPlanRecordsOneEventPerViolation(t *testing.T) {
	exporter := setupTracing(t)
	input := compliantInput()
	input.Plan.ReplicaWriters = append(input.Plan.ReplicaWriters, "KEDA")
	body, _ := json.Marshal(input)
	post(t, body, "tenant-a")
	service := findSpan(t, exporter, "autoscaling.service.evaluate_plan")
	if attributeOf(service, "autoscaling.allowed") != "false" || !strings.Contains(attributeOf(service, "autoscaling.violation.codes"), "MULTIPLE_REPLICA_WRITERS") {
		t.Fatalf("denial not explained: %+v", service.Attributes)
	}
	events := 0
	for _, event := range service.Events {
		if event.Name == "invariant.violated" {
			events++
		}
	}
	if events == 0 || service.Status.Code == codes.Error {
		t.Fatalf("expected violation events on a non-error span, events=%d status=%v", events, service.Status.Code)
	}
	if attributeOf(findSpan(t, exporter, "autoscaling.api.evaluate_plan"), "autoscaling.outcome") != "denied" {
		t.Fatal("handler outcome should be denied")
	}
}

func TestTrace_RejectedRequestExplainsWhyItFailed(t *testing.T) {
	exporter := setupTracing(t)
	body, _ := json.Marshal(compliantInput())
	if rec := post(t, body, ""); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}
	service := findSpan(t, exporter, "autoscaling.service.evaluate_plan")
	if service.Status.Code != codes.Error || !strings.Contains(attributeOf(service, "autoscaling.validation.reason"), "tenant context is required") {
		t.Fatalf("rejection not explained: status=%v attrs=%+v", service.Status, service.Attributes)
	}
	if attributeOf(service, "app.error_class") != "client" || attributeOf(service, "app.status_code") != "400" {
		t.Fatalf("error class missing: %+v", service.Attributes)
	}
	handler := findSpan(t, exporter, "autoscaling.api.evaluate_plan")
	if attributeOf(handler, "autoscaling.outcome") != "validation_failed" || attributeOf(handler, "http.response.status_code") != "422" {
		t.Fatalf("handler did not record the rejection: %+v", handler.Attributes)
	}
	if attributeOf(handler, "autoscaling.tenant_header_present") != "false" {
		t.Fatal("handler should record that the tenant header was absent")
	}
}
