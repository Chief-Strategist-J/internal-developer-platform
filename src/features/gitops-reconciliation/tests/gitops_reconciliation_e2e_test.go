package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/routers"
	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/infrastructure/adapters"
	gitopsService "internal-developer-platform/src/features/gitops-reconciliation/service"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/types"
)

func setupE2EGitOpsServer(t *testing.T) (http.Handler, *tracetest.InMemoryExporter, func()) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	store := adapters.NewMemoryGitOpsRepositoryAdapter()
	argo := adapters.NewArgoCDClientAdapter("http://localhost:8085", "test-token")
	svc := gitopsService.NewGitOpsReconciliationService(store, argo)
	svc.SetSecretAdapter(adapters.NewConfigurableSecretAdapter())

	handler := handlers.NewGitOpsReconciliationHandler(svc)
	router := routers.NewGitOpsReconciliationRouter(handler, nil)

	mux := http.NewServeMux()
	router.RegisterRoutes(mux)

	wrapped := observability.IngressTelemetryMiddleware(mux)

	cleanup := func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	}
	return wrapped, exporter, cleanup
}

func TestGitOpsReconciliationE2ETraceHierarchyAndPropagation(t *testing.T) {
	server, exporter, cleanup := setupE2EGitOpsServer(t)
	defer cleanup()

	body, err := json.Marshal(contracts.SyncCommand{
		ID:                 "rec-e2e-001",
		TenantID:           "tenant-e2e-1",
		ServiceID:          "svc-orders",
		AppName:            "orders-dev",
		TargetRevision:     "abc1234",
		ProjectEnforcesSig: true,
		CommitSigned:       true,
		AllowedSigningKey:  true,
		RoleReviewed:       true,
		InAllowWindow:      true,
		IdempotencyKey:     "idem-e2e-001",
	})
	if err != nil {
		t.Fatalf("failed to marshal sync command: %v", err)
	}

	traceIDHex := "4bf92f3577b34da6a3ce929d0e0e4736"
	parentSpanHex := "00f067aa0ba902b7"
	traceparent := "00-" + traceIDHex + "-" + parentSpanHex + "-01"

	req := httptest.NewRequest(http.MethodPost, constants.EndpointReconciliations, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(constants.HeaderXTenantID, "tenant-e2e-1")
	req.Header.Set(constants.HeaderXCorrelationID, "corr-e2e-001")
	req.Header.Set(constants.HeaderTraceparent, traceparent)

	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected HTTP 202 Accepted, got %d: %s", rr.Code, rr.Body.String())
	}

	if gotTraceID := rr.Header().Get("X-Trace-Id"); gotTraceID != traceIDHex {
		t.Fatalf("expected X-Trace-Id %s, got %s", traceIDHex, gotTraceID)
	}
	if gotSpanID := rr.Header().Get("X-Span-Id"); gotSpanID == "" {
		t.Fatal("expected non-empty X-Span-Id response header")
	}

	var apiResp types.ApiResponse
	if err := json.NewDecoder(rr.Body).Decode(&apiResp); err != nil {
		t.Fatalf("failed to decode api response: %v", err)
	}
	if apiResp.Meta.CorrelationID != "corr-e2e-001" {
		t.Fatalf("expected correlationId corr-e2e-001, got %s", apiResp.Meta.CorrelationID)
	}

	spans := exporter.GetSpans()
	spanNames := make(map[string]bool, len(spans))
	for _, s := range spans {
		spanNames[s.Name] = true
		if s.SpanContext.TraceID().String() != traceIDHex {
			t.Fatalf("span %s broke trace context propagation: got %s, want %s", s.Name, s.SpanContext.TraceID().String(), traceIDHex)
		}
	}

	expectedSpans := []string{
		"POST " + constants.EndpointReconciliations,
		"gitops_reconciliation.trigger_sync",
		"gitops.reconciliation.trigger",
		"gitops.workflow.execute_plan",
		"gitops.workflow.fetch_manifests",
		"gitops.workflow.apply_sync_waves",
		"gitops.workflow.verify_health",
		"memory.gitops.save",
		"argocd.diff_manifests",
		"argocd.trigger_sync",
		"argocd.get_application_status",
		"memory.gitops.update",
	}
	for _, name := range expectedSpans {
		if !spanNames[name] {
			t.Fatalf("expected span %q in trace hierarchy, got %v", name, spanNames)
		}
	}
}

func TestGitOpsReconciliationE2EFailureRecordsErrorOnSpan(t *testing.T) {
	server, exporter, cleanup := setupE2EGitOpsServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, constants.EndpointReconciliations, bytes.NewBufferString(`{"invalid":`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(constants.HeaderXTenantID, "tenant-e2e-err")

	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400 Bad Request, got %d", rr.Code)
	}

	spans := exporter.GetSpans()
	foundErrorSpan := false
	for _, s := range spans {
		if s.Name == "gitops_reconciliation.trigger_sync" && s.Status.Code == codes.Error {
			foundErrorSpan = true
			break
		}
	}
	if !foundErrorSpan {
		t.Fatal("expected gitops_reconciliation.trigger_sync span to be marked codes.Error on 400 rejection")
	}
}
