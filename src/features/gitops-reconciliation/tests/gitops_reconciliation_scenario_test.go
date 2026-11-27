package tests

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"internal-developer-platform/src/features/gitops-reconciliation/internal/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/domain/rules"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/infrastructure/adapters"
	gitopsService "internal-developer-platform/src/features/gitops-reconciliation/internal/service"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/errors"
)

func TestGitOpsReconciliationAdversarialIdempotentReplay(t *testing.T) {
	store := adapters.NewMemoryGitOpsRepositoryAdapter()
	argo := adapters.NewArgoCDClientAdapter("http://localhost:8085", "token")
	svc := gitopsService.NewGitOpsReconciliationService(store, argo)

	cmd := contracts.SyncCommand{
		ID:                 "rec-replay-001",
		TenantID:           "tenant-scen-1",
		ServiceID:          "svc-billing",
		AppName:            "billing-dev",
		TargetRevision:     "rev-101",
		ProjectEnforcesSig: true,
		CommitSigned:       true,
		AllowedSigningKey:  true,
		RoleReviewed:       true,
		InAllowWindow:      true,
		IdempotencyKey:     "idem-replay-001",
	}

	first, err := svc.TriggerReconciliation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("first reconciliation failed: %v", err)
	}
	if first.Reconciliation.SyncPhase != contracts.SyncPhaseSynced {
		t.Fatalf("expected SyncPhaseSynced, got %s", first.Reconciliation.SyncPhase)
	}

	second, err := svc.TriggerReconciliation(context.Background(), cmd)
	if err != nil {
		t.Fatalf("idempotent replay failed: %v", err)
	}
	if second.Message != "idempotent replay" {
		t.Fatalf("expected idempotent replay message, got %q", second.Message)
	}
}

func TestGitOpsReconciliationAdversarialStaleCASConflict(t *testing.T) {
	store := adapters.NewMemoryGitOpsRepositoryAdapter()
	now := time.Now().UTC()

	rec := contracts.ReconciliationRecord{
		ID:             "rec-cas-001",
		TenantID:       "tenant-cas",
		ServiceID:      "svc-cas",
		AppName:        "cas-dev",
		TargetRevision: "rev-1",
		SyncPhase:      contracts.SyncPhasePending,
		HealthStatus:   contracts.HealthStatusProgressing,
		IdempotencyKey: "idem-cas-001",
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := store.Save(context.Background(), rec); err != nil {
		t.Fatalf("failed to seed record: %v", err)
	}

	rec.Version = 2
	if err := store.Update(context.Background(), rec); err != nil {
		t.Fatalf("expected version 2 update to succeed: %v", err)
	}

	stale := rec
	stale.Version = 2
	err := store.Update(context.Background(), stale)
	if err == nil {
		t.Fatal("expected stale CAS update to return conflict error")
	}
	if !errors.IsConflictError(err) {
		t.Fatalf("expected ConflictError, got %T: %v", err, err)
	}
}

func TestGitOpsReconciliationAdversarialEdgeCases(t *testing.T) {
	server, _, cleanup := setupE2EGitOpsServer(t)
	defer cleanup()

	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{
			name:       "missing required fields returns 400",
			method:     http.MethodPost,
			path:       constants.EndpointReconciliations,
			body:       `{"appName":""}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "non-existent reconciliation returns 404",
			method:     http.MethodGet,
			path:       constants.EndpointReconciliationsPrefix + "does-not-exist",
			body:       "",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "non-existent drift diff returns 404",
			method:     http.MethodGet,
			path:       constants.EndpointReconciliationsPrefix + "does-not-exist/diff",
			body:       "",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "unknown field in secret config returns 400",
			method:     http.MethodPut,
			path:       constants.EndpointGitOpsSecretsConfig,
			body:       `{"tenantId":"t1","mechanism":"SEALED_SECRETS","unexpectedField":"bad"}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "malformed webhook payload returns 400",
			method:     http.MethodPost,
			path:       constants.EndpointGitOpsWebhook,
			body:       `{"repositoryUrl":""}`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reader *bytes.Reader
			if tc.body != "" {
				reader = bytes.NewReader([]byte(tc.body))
			} else {
				reader = bytes.NewReader(nil)
			}
			req := httptest.NewRequest(tc.method, tc.path, reader)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(constants.HeaderXTenantID, "tenant-edge")

			rr := httptest.NewRecorder()
			server.ServeHTTP(rr, req)

			if rr.Code != tc.wantStatus {
				t.Fatalf("%s: expected status %d, got %d (%s)", tc.name, tc.wantStatus, rr.Code, strings.TrimSpace(rr.Body.String()))
			}
		})
	}
}

func TestGitOpsReconciliationGuardrailBlocksClusterScopedResource(t *testing.T) {
	if err := rules.ValidateTenantProjectGuardrails(true, "ClusterRole"); err == nil {
		t.Fatal("expected guardrail to reject ClusterRole in tenant-scoped project")
	}
}
