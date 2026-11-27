package tests

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	restv1 "internal-developer-platform/src/features/role-based-access-control/internal/api/rest/v1"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/internal/contracts"
	"internal-developer-platform/src/features/role-based-access-control/internal/infrastructure/adapters"
	rbacService "internal-developer-platform/src/features/role-based-access-control/internal/service"
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

func TestRoleBasedAccessControlE2E_IngressEnforcesABACFreeze(t *testing.T) {
	svc, _, cleanup := setupE2ERBACService(t)
	defer cleanup()

	ctx := context.Background()
	sub := rbacContracts.Subject{
		ID:        "dev-abac-1",
		TenantID:  "tenant-e2e",
		Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	token, err := svc.IssueToken(sub)
	if err != nil {
		t.Fatalf("IssueToken failed: %v", err)
	}

	_, err = svc.AuthenticateAndAuthorizeWithContext(ctx, token, rbacContracts.EvaluationRequest{
		TargetTenantID:     "tenant-e2e",
		Resource:           rbacContracts.ResourceServiceCatalog,
		Action:             rbacContracts.ActionWrite,
		TargetEnvironment:  rbacContracts.EnvProduction,
		ChangeFreezeActive: true,
	})
	if err == nil {
		t.Fatal("expected AuthenticateAndAuthorizeWithContext to reject production write during change freeze")
	}
}

func TestRoleBasedAccessControlE2E_NewPhase3EndpointsAndTracing(t *testing.T) {
	svc, exporter, cleanup := setupE2ERBACService(t)
	defer cleanup()

	verifier := adapters.NewJWTTokenVerifierAdapter("e2e-test-secret-key", "idp-e2e")
	revocation := adapters.NewMemoryRevocationAdapter()
	grantRepo := adapters.NewMemoryAccessGrantAdapter()
	bootstrappedSvc := rbacService.NewRBACServiceWithDeps(rbacService.RBACServiceDeps{
		TokenVerifier:   verifier,
		RevocationStore: revocation,
		GrantRepo:       grantRepo,
		BootstrapSecret: "test-bootstrap-secret",
	})

	ctx := context.Background()
	if err := bootstrappedSvc.VerifyTokenIssuanceAccess(ctx, "test-bootstrap-secret", ""); err != nil {
		t.Fatalf("expected valid bootstrap secret to pass, got %v", err)
	}
	if err := bootstrappedSvc.VerifyTokenIssuanceAccess(ctx, "wrong-secret", ""); err == nil {
		t.Fatal("expected wrong bootstrap secret to be rejected")
	}

	handler := restv1.NewRBACHandler(svc)

	leaseReq := httptest.NewRequest(http.MethodPost, "/api/v1/rbac/leases/evaluate", bytes.NewBufferString(`{"tokenExpired":false,"isIdempotentBatchStep":true,"hasSignedTaskLease":true}`))
	leaseRec := httptest.NewRecorder()
	handler.EvaluateTaskLease(leaseRec, leaseReq)
	if leaseRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for valid task lease, got %d", leaseRec.Code)
	}

	leaseFailReq := httptest.NewRequest(http.MethodPost, "/api/v1/rbac/leases/evaluate", bytes.NewBufferString(`{"tokenExpired":true,"isIdempotentBatchStep":true,"hasSignedTaskLease":true,"leaseExpired":true}`))
	leaseFailRec := httptest.NewRecorder()
	handler.EvaluateTaskLease(leaseFailRec, leaseFailReq)
	if leaseFailRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for expired task lease, got %d", leaseFailRec.Code)
	}

	driftReq := httptest.NewRequest(http.MethodPost, "/api/v1/rbac/jobs/evaluate-drift", bytes.NewBufferString(`{"subjectId":"worker-1","jti":"jti-1","rawToken":"tok","queueLatencySeconds":10}`))
	driftRec := httptest.NewRecorder()
	handler.EvaluateAsyncJobDrift(driftRec, driftReq)
	if driftRec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for low-latency async job drift, got %d", driftRec.Code)
	}

	driftFailReq := httptest.NewRequest(http.MethodPost, "/api/v1/rbac/jobs/evaluate-drift", bytes.NewBufferString(`{"subjectId":"worker-1","jti":"jti-1","rawToken":"tok","queueLatencySeconds":3600}`))
	driftFailRec := httptest.NewRecorder()
	handler.EvaluateAsyncJobDrift(driftFailRec, driftFailReq)
	if driftFailRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for stale async job drift without OIDC, got %d", driftFailRec.Code)
	}

	streamReq := httptest.NewRequest(http.MethodGet, "/api/v1/rbac/sessions/stream", nil)
	streamRec := httptest.NewRecorder()
	handler.StreamTerminalSession(streamRec, streamReq)
	if streamRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for unauthenticated stream session, got %d", streamRec.Code)
	}

	sub := rbacContracts.Subject{
		ID:        "dev-split-1",
		TenantID:  "tenant-e2e",
		Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	if _, err := svc.AuthorizeWithContext(ctx, rbacContracts.EvaluationRequest{
		Subject:             sub,
		TargetTenantID:      "tenant-e2e",
		Resource:            rbacContracts.ResourceServiceCatalog,
		Action:              rbacContracts.ActionWrite,
		GlobalAuthorityDown: true,
	}); err == nil {
		t.Fatal("expected split-brain guard to reject mutation when global authority is down")
	}

	spans := exporter.GetSpans()
	seen := map[string]bool{
		"rbac.service.verify_token_issuance_access": false,
		"rbac.handler.evaluate_task_lease":          false,
		"rbac.service.evaluate_task_lease":          false,
		"rbac.handler.evaluate_async_job_drift":     false,
		"rbac.service.evaluate_async_job_drift":     false,
		"rbac.handler.stream_terminal_session":      false,
		"rbac.service.evaluate_split_brain":         false,
	}
	errorSeen := map[string]bool{
		"rbac.service.verify_token_issuance_access": false,
		"rbac.handler.evaluate_task_lease":          false,
		"rbac.handler.evaluate_async_job_drift":     false,
		"rbac.handler.stream_terminal_session":      false,
		"rbac.service.evaluate_split_brain":         false,
	}
	for _, sp := range spans {
		if _, ok := seen[sp.Name]; ok {
			seen[sp.Name] = true
		}
		if _, ok := errorSeen[sp.Name]; ok && sp.Status.Code == codes.Error {
			errorSeen[sp.Name] = true
		}
	}
	for name, ok := range seen {
		if !ok {
			t.Errorf("expected span %s to be emitted", name)
		}
	}
	for name, ok := range errorSeen {
		if !ok {
			t.Errorf("expected error span %s to be marked with codes.Error", name)
		}
	}
}
