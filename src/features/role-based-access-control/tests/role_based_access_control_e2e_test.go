package tests

import (
	"context"
	"net/http"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
)

func setupE2ERBACService(t *testing.T) (*rbacService.RBACService, *tracetest.InMemoryExporter, func()) {
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

	verifier := adapters.NewJWTTokenVerifierAdapter("e2e-test-secret-key", "idp-e2e")
	revocation := adapters.NewMemoryRevocationAdapter()
	grantRepo := adapters.NewMemoryAccessGrantAdapter()
	bindingRepo := adapters.NewMemoryUserBindingAdapter()
	auditOutbox := adapters.NewMemoryRBACAuditOutboxAdapter()

	svc := rbacService.NewRBACService(verifier, revocation, grantRepo)
	svc.SetUserBindingRepository(bindingRepo)
	svc.SetAuditOutbox(auditOutbox)

	cleanup := func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	}
	return svc, exporter, cleanup
}

func TestRoleBasedAccessControlE2E_TraceHierarchyAndPropagation(t *testing.T) {
	svc, exporter, cleanup := setupE2ERBACService(t)
	defer cleanup()

	expectedTraceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	traceparent := "00-" + expectedTraceID + "-00f067aa0ba902b7-01"
	headers := http.Header{}
	headers.Set("traceparent", traceparent)
	ctx := otel.GetTextMapPropagator().Extract(context.Background(), propagation.HeaderCarrier(headers))

	subject := rbacContracts.Subject{
		ID:        "user-e2e-1",
		TenantID:  "tenant-e2e",
		Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
		ExpiresAt: time.Now().Add(1 * time.Hour),
		JTI:       "jti-e2e-1",
	}

	token, err := svc.IssueToken(subject)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	authed, err := svc.AuthenticateAndAuthorize(ctx, token, "tenant-e2e", rbacContracts.ResourceServiceCatalog, rbacContracts.ActionRead)
	if err != nil {
		t.Fatalf("AuthenticateAndAuthorize failed: %v", err)
	}
	if authed.ID != subject.ID {
		t.Fatalf("expected subject %s, got %s", subject.ID, authed.ID)
	}

	grantReq := rbacContracts.AccessGrantRequest{
		ID:              "grant-e2e-1",
		TenantID:        "tenant-e2e",
		SubjectID:       "user-e2e-1",
		TargetNamespace: "prod-ns",
		Role:            string(rbacContracts.RoleDeveloper),
		Reason:          "e2e incident response",
		DurationSeconds: 1800,
		ApproverRole:    string(rbacContracts.RolePlatformAdmin),
		IdempotencyKey:  "idem-e2e-1",
	}
	if _, err := svc.RequestJITAccess(ctx, grantReq); err != nil {
		t.Fatalf("RequestJITAccess failed: %v", err)
	}

	grants, err := svc.ListActiveGrants(ctx, "tenant-e2e", "user-e2e-1")
	if err != nil || len(grants) != 1 {
		t.Fatalf("ListActiveGrants failed: len=%d err=%v", len(grants), err)
	}

	if err := svc.RevokeJITAccess(ctx, "tenant-e2e", "grant-e2e-1"); err != nil {
		t.Fatalf("RevokeJITAccess failed: %v", err)
	}

	spans := exporter.GetSpans()
	spanByName := make(map[string]tracetest.SpanStub)
	for _, sp := range spans {
		spanByName[sp.Name] = sp
	}

	requiredSpans := []string{
		"rbac.service.authenticate_and_authorize",
		"rbac.service.authenticate_token",
		"rbac.service.authorize",
		"rbac.service.request_jit_access",
		"rbac.service.list_active_grants",
		"rbac.service.revoke_jit_access",
	}
	for _, name := range requiredSpans {
		sp, ok := spanByName[name]
		if !ok {
			t.Fatalf("expected span %s to be recorded", name)
		}
		if sp.SpanContext.TraceID().String() != expectedTraceID {
			t.Fatalf("span %s traceID mismatch: got %s want %s", name, sp.SpanContext.TraceID().String(), expectedTraceID)
		}
	}

	parentSpan := spanByName["rbac.service.authenticate_and_authorize"]
	authTokenSpan := spanByName["rbac.service.authenticate_token"]
	authorizeSpan := spanByName["rbac.service.authorize"]

	if authTokenSpan.Parent.SpanID() != parentSpan.SpanContext.SpanID() {
		t.Fatalf("expected authenticate_token parent %s, got %s", parentSpan.SpanContext.SpanID(), authTokenSpan.Parent.SpanID())
	}
	if authorizeSpan.Parent.SpanID() != parentSpan.SpanContext.SpanID() {
		t.Fatalf("expected authorize parent %s, got %s", parentSpan.SpanContext.SpanID(), authorizeSpan.Parent.SpanID())
	}
}

func TestRoleBasedAccessControlE2E_ErrorSpanRecording(t *testing.T) {
	svc, exporter, cleanup := setupE2ERBACService(t)
	defer cleanup()

	ctx := context.Background()

	if _, err := svc.AuthenticateAndAuthorize(ctx, "invalid.jwt.token", "tenant-e2e", rbacContracts.ResourceServiceCatalog, rbacContracts.ActionRead); err == nil {
		t.Fatal("expected error for invalid token")
	}

	if err := svc.RevokeJITAccess(ctx, "", ""); err == nil {
		t.Fatal("expected validation error for empty tenantId and grantId")
	}

	spans := exporter.GetSpans()
	errorSpans := 0
	for _, sp := range spans {
		if sp.Status.Code == codes.Error {
			errorSpans++
		}
	}
	if errorSpans < 2 {
		t.Fatalf("expected at least 2 error spans recorded, got %d", errorSpans)
	}
}
