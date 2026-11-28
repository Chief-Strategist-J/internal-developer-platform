/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING API TESTS (V.3 role 14)

 1. Role:
    - Verifies cluster provisioning, posture validation, workload identity manifests, and ledger reconciliation endpoints.
*/
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	restv1 "internal-developer-platform/src/features/self-service-provisioning/internal/api/rest/v1"
	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/features/self-service-provisioning/internal/domain/rules"
	"internal-developer-platform/src/features/self-service-provisioning/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/self-service-provisioning/internal/service"
	"internal-developer-platform/src/infra/clients/cloudresource"
	"internal-developer-platform/src/infra/clients/cluster"
	"internal-developer-platform/src/infra/clients/resilience"
	"internal-developer-platform/src/infra/middleware"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/errors"
	"internal-developer-platform/src/shared/types"
)

func createTestClusterSpec(id string) cluster.GKEClusterSpec {
	return cluster.GKEClusterSpec{
		ClusterID:      id,
		ProjectID:      "gov-cloud-prod",
		Location:       "us-central1",
		ReleaseChannel: cluster.ReleaseChannelRegular,
		WorkloadPool:   "gov-cloud-prod" + cluster.WorkloadPoolSuffix,
		PrivateCluster: cluster.PrivateClusterConfig{
			EnablePrivateNodes:    true,
			EnablePrivateEndpoint: true,
			MasterIPv4CIDRBlock:   "172.16.0.0/28",
		},
		ShieldedNodes: cluster.ShieldedNodesConfig{
			Enabled:             true,
			SecureBoot:          true,
			IntegrityMonitoring: true,
		},
		DatabaseEncryption: cluster.DatabaseEncryptionConfig{
			State:   cluster.DatabaseEncryptionStateEncrypted,
			KeyName: "projects/gov-cloud-prod/locations/us-central1/keyRings/ring/cryptoKeys/key",
		},
		BinaryAuthorization: cluster.BinaryAuthorizationConfig{
			EvaluationMode: cluster.BinaryAuthModeEnforce,
		},
		BackupAgentEnabled:   true,
		CloudServiceMeshMode: cluster.MeshModeManaged,
		DataAccessLogsActive: true,
	}
}

func newTestHandler() *restv1.SelfServiceProvisioningHandler {
	resCfg := resilience.DefaultResilienceConfig()
	clAdapter := adapters.NewGKEClusterClientAdapter(cluster.NewGKEClusterClient(resCfg))
	crAdapter := adapters.NewCloudResourceClientAdapter(cloudresource.NewCloudResourceClient(resCfg))
	svc := service.NewSelfServiceProvisioningService(clAdapter, crAdapter)
	return restv1.NewSelfServiceProvisioningHandler(svc)
}

func TestSelfServiceCluster_ProvisionAndValidatePosture(t *testing.T) {
	handler := newTestHandler()
	spec := createTestClusterSpec("cluster-test-01")
	body, _ := json.Marshal(spec)

	provReq := httptest.NewRequest(http.MethodPost, constants.EndpointClusters, bytes.NewReader(body))
	provRec := httptest.NewRecorder()
	handler.ProvisionCluster(provRec, provReq)

	if provRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got: %d", provRec.Code)
	}

	postureReq := httptest.NewRequest(http.MethodGet, constants.EndpointClustersPosture+"?clusterId=cluster-test-01", nil)
	postureRec := httptest.NewRecorder()
	handler.ValidatePosture(postureRec, postureReq)

	if postureRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got: %d", postureRec.Code)
	}

	var resp types.ApiResponse
	if err := json.NewDecoder(postureRec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response")
	}
}

func TestSelfServiceCloudResource_ReconcileLedger(t *testing.T) {
	handler := newTestHandler()
	spec := cloudresource.WorkloadIdentityBindingSpec{
		GSAEmail:     "app@gov-cloud.iam.gserviceaccount.com",
		KSAName:      "app-sa",
		Namespace:    "app-ns",
		ProjectID:    "gov-cloud",
		RoleBindings: []string{"roles/storage.objectViewer"},
		Metadata: cloudresource.ResourceMetadata{
			RequestID:   "req-1",
			Owner:       "team@gov.io",
			CostCenter:  "CC-1",
			WorkloadID:  "app-workload",
			Environment: "production",
		},
	}
	body, _ := json.Marshal(spec)

	provReq := httptest.NewRequest(http.MethodPost, constants.EndpointCloudResources, bytes.NewReader(body))
	provRec := httptest.NewRecorder()
	handler.ProvisionResource(provRec, provReq)

	if provRec.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got: %d", provRec.Code)
	}

	reconBody, _ := json.Marshal(map[string][]string{
		"activeWorkloadIds": {"other-workload"},
	})
	reconReq := httptest.NewRequest(http.MethodPost, constants.EndpointCloudResourcesReconcile, bytes.NewReader(reconBody))
	reconRec := httptest.NewRecorder()
	handler.ReconcileLedger(reconRec, reconReq)

	if reconRec.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK, got: %d", reconRec.Code)
	}

	var resp types.ApiResponse
	if err := json.NewDecoder(reconRec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success {
		t.Fatalf("expected success response")
	}
}

func TestSelfServiceProvisioning_ErrorPathsLedgerAndMultiSpanTracing(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter), sdktrace.WithSampler(sdktrace.AlwaysSample()))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	}()

	resCfg := resilience.DefaultResilienceConfig()
	clAdapter := adapters.NewGKEClusterClientAdapter(cluster.NewGKEClusterClient(resCfg))
	crAdapter := adapters.NewCloudResourceClientAdapter(cloudresource.NewCloudResourceClient(resCfg))
	ledger := adapters.NewMemoryProvisioningLedgerAdapter()
	svc := service.NewSelfServiceProvisioningService(clAdapter, crAdapter)
	svc.SetLedgerPort(ledger)
	handler := restv1.NewSelfServiceProvisioningHandler(svc)

	spec := createTestClusterSpec("cluster-traced-01")
	body, _ := json.Marshal(spec)
	provReq := httptest.NewRequest(http.MethodPost, constants.EndpointClusters, bytes.NewReader(body))
	provReq.Header.Set("X-Tenant-Id", "tenant-ssp-trace")
	provRec := httptest.NewRecorder()
	handler.ProvisionCluster(provRec, provReq)
	if provRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", provRec.Code)
	}

	if len(ledger.Intents) != 1 {
		t.Fatalf("expected 1 INV-01 intent recorded in ledger, got %d", len(ledger.Intents))
	}
	for _, it := range ledger.Intents {
		if it.Status != "READY" || it.OutcomeID != "cluster-traced-01" {
			t.Fatalf("expected READY intent with outcome cluster-traced-01, got status=%s outcome=%s", it.Status, it.OutcomeID)
		}
	}

	notFoundReq := httptest.NewRequest(http.MethodGet, constants.EndpointClusters+"?clusterId=missing-cluster", nil)
	notFoundReq.Header.Set("X-Tenant-Id", "tenant-ssp-trace")
	notFoundRec := httptest.NewRecorder()
	handler.GetCluster(notFoundRec, notFoundReq)
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found, got %d", notFoundRec.Code)
	}

	badReq := httptest.NewRequest(http.MethodPost, constants.EndpointClusters, bytes.NewBufferString(`{"invalid_json"`))
	badRec := httptest.NewRecorder()
	handler.ProvisionCluster(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", badRec.Code)
	}

	spans := exporter.GetSpans()
	seen := map[string]bool{
		"self_service.cluster.provision":              false,
		"self_service.service.provision_cluster":      false,
		"self_service.ledger.memory.record_intent":    false,
		"self_service.client.gke.provision_cluster":   false,
		"self_service.ledger.memory.record_outcome":   false,
		"self_service.cluster.get":                    false,
		"self_service.service.get_cluster":            false,
		"self_service.client.gke.get_cluster":         false,
	}
	errorSeen := map[string]bool{
		"self_service.cluster.provision": false,
	}
	notFoundSpans := map[string]bool{
		"self_service.cluster.get":            true,
		"self_service.service.get_cluster":    true,
		"self_service.client.gke.get_cluster": true,
	}
	for _, sp := range spans {
		if _, ok := seen[sp.Name]; ok {
			seen[sp.Name] = true
		}
		if _, ok := errorSeen[sp.Name]; ok && sp.Status.Code == codes.Error {
			errorSeen[sp.Name] = true
		}
		if notFoundSpans[sp.Name] && sp.Status.Code == codes.Error {
			t.Errorf("expected 404 span %s not to be marked with codes.Error", sp.Name)
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

type failingIntentLedger struct{}

func (f *failingIntentLedger) RecordIntent(_ context.Context, _ contracts.ProvisioningIntent) error {
	return errors.NewInternalError("ledger unavailable")
}

func (f *failingIntentLedger) RecordOutcome(_ context.Context, _ string, _ int, _ string, _ string, _ string) error {
	return nil
}

func (f *failingIntentLedger) ListIntents(_ context.Context, _ string, _ int) ([]contracts.ProvisioningIntent, error) {
	return nil, nil
}

func TestSelfService_INV01_IntentFailureAbortsBeforeEffect(t *testing.T) {
	resCfg := resilience.DefaultResilienceConfig()
	clAdapter := adapters.NewGKEClusterClientAdapter(cluster.NewGKEClusterClient(resCfg))
	crAdapter := adapters.NewCloudResourceClientAdapter(cloudresource.NewCloudResourceClient(resCfg))
	svc := service.NewSelfServiceProvisioningService(clAdapter, crAdapter)
	svc.SetLedgerPort(&failingIntentLedger{})

	_, err := svc.ProvisionCluster(context.Background(), createTestClusterSpec("cluster-inv01-abort"))
	if err == nil {
		t.Fatal("expected ProvisionCluster to fail when intent persistence fails")
	}
	if _, getErr := svc.GetCluster(context.Background(), "cluster-inv01-abort"); getErr == nil {
		t.Fatal("expected cluster not to be created when intent persistence failed")
	}
}

func TestSelfService_INV03_StaleAttemptFenced(t *testing.T) {
	ledger := adapters.NewMemoryProvisioningLedgerAdapter()
	err := ledger.RecordIntent(context.Background(), contracts.ProvisioningIntent{
		ID:           "intent-fence-01",
		PublicID:     "sspi_0123456789abcdef0123456789",
		TenantID:     "tenant-a",
		ResourceKind: "CLUSTER",
		TargetID:     "cluster-fence-01",
		Attempt:      2,
		Status:       "PENDING",
	})
	if err != nil {
		t.Fatalf("unexpected RecordIntent error: %v", err)
	}
	staleErr := ledger.RecordOutcome(context.Background(), "intent-fence-01", 1, "READY", "cluster-fence-01", "")
	if !errors.IsConflictError(staleErr) {
		t.Fatalf("expected conflict error for stale attempt 1, got: %v", staleErr)
	}
	if okErr := ledger.RecordOutcome(context.Background(), "intent-fence-01", 2, "READY", "cluster-fence-01", ""); okErr != nil {
		t.Fatalf("expected attempt 2 to succeed, got: %v", okErr)
	}
}

func TestSelfService_EC02_IdempotencyReplayAndMismatch(t *testing.T) {
	handler := newTestHandler()
	idempMW := middleware.NewIdempotencyMiddleware()
	router := restv1.NewSelfServiceProvisioningRouter(handler, nil, idempMW)
	mux := http.NewServeMux()
	router.RegisterRoutes(mux)

	spec1 := createTestClusterSpec("cluster-ec02-01")
	body1, _ := json.Marshal(spec1)

	req1 := httptest.NewRequest(http.MethodPost, constants.EndpointClusters, bytes.NewReader(body1))
	req1.Header.Set("X-Tenant-Id", "tenant-ec02")
	req1.Header.Set("X-User-Id", "user-ec02")
	req1.Header.Set("Idempotency-Key", "idem-key-01")
	rec1 := httptest.NewRecorder()
	mux.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on first submit, got %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, constants.EndpointClusters, bytes.NewReader(body1))
	req2.Header.Set("X-Tenant-Id", "tenant-ec02")
	req2.Header.Set("X-User-Id", "user-ec02")
	req2.Header.Set("Idempotency-Key", "idem-key-01")
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on idempotent replay, got %d", rec2.Code)
	}
	if rec2.Body.String() != rec1.Body.String() {
		t.Fatalf("expected replayed body to match original response")
	}

	spec2 := createTestClusterSpec("cluster-ec02-02")
	body2, _ := json.Marshal(spec2)
	req3 := httptest.NewRequest(http.MethodPost, constants.EndpointClusters, bytes.NewReader(body2))
	req3.Header.Set("X-Tenant-Id", "tenant-ec02")
	req3.Header.Set("X-User-Id", "user-ec02")
	req3.Header.Set("Idempotency-Key", "idem-key-01")
	rec3 := httptest.NewRecorder()
	mux.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity on payload mismatch, got %d", rec3.Code)
	}
}

func TestSelfService_EC03_NameRaceReturns409Conflict(t *testing.T) {
	resCfg := resilience.DefaultResilienceConfig()
	clAdapter := adapters.NewGKEClusterClientAdapter(cluster.NewGKEClusterClient(resCfg))
	crAdapter := adapters.NewCloudResourceClientAdapter(cloudresource.NewCloudResourceClient(resCfg))
	svc := service.NewSelfServiceProvisioningService(clAdapter, crAdapter)
	svc.SetReservationPort(adapters.NewMemoryProvisioningReservationAdapter())
	handler := restv1.NewSelfServiceProvisioningHandler(svc)

	specA := createTestClusterSpec("Cluster-Race-01")
	bodyA, _ := json.Marshal(specA)
	reqA := httptest.NewRequest(http.MethodPost, constants.EndpointClusters, bytes.NewReader(bodyA))
	reqA.Header.Set("X-Tenant-Id", "tenant-alpha")
	recA := httptest.NewRecorder()
	handler.ProvisionCluster(recA, reqA)
	if recA.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for first tenant, got %d", recA.Code)
	}

	specB := createTestClusterSpec("  cluster-race-01  ")
	bodyB, _ := json.Marshal(specB)
	reqB := httptest.NewRequest(http.MethodPost, constants.EndpointClusters, bytes.NewReader(bodyB))
	reqB.Header.Set("X-Tenant-Id", "tenant-beta")
	recB := httptest.NewRecorder()
	handler.ProvisionCluster(recB, reqB)
	if recB.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict for second tenant racing normalized name, got %d", recB.Code)
	}
}

func TestSelfService_INV04_INV05_INV15_CapacityAndIPCeilings(t *testing.T) {
	resAdapter := adapters.NewMemoryProvisioningReservationAdapter()
	if err := resAdapter.SetTenantCeiling(context.Background(), contracts.TenantCapacityCeiling{
		TenantID:            "tenant-capped",
		MaxQuotaUnits:       1,
		MaxIPCIDRBlocks:     1,
		MaxClusterIPUtilPct: 80,
	}); err != nil {
		t.Fatalf("unexpected SetTenantCeiling error: %v", err)
	}

	resCfg := resilience.DefaultResilienceConfig()
	clAdapter := adapters.NewGKEClusterClientAdapter(cluster.NewGKEClusterClient(resCfg))
	crAdapter := adapters.NewCloudResourceClientAdapter(cloudresource.NewCloudResourceClient(resCfg))
	svc := service.NewSelfServiceProvisioningService(clAdapter, crAdapter)
	svc.SetReservationPort(resAdapter)

	_, err := svc.ProvisionCluster(context.Background(), func() contracts.ClusterSpec {
		s := createTestClusterSpec("cluster-cap-01")
		s.ProjectID = "tenant-capped"
		return s
	}())
	if err != nil {
		t.Fatalf("expected first cluster within ceiling to succeed, got: %v", err)
	}

	_, secondErr := svc.ProvisionCluster(context.Background(), func() contracts.ClusterSpec {
		s := createTestClusterSpec("cluster-cap-02")
		s.ProjectID = "tenant-capped"
		return s
	}())
	if !errors.IsConflictError(secondErr) {
		t.Fatalf("expected conflict error when exceeding tenant ceiling, got: %v", secondErr)
	}
	if _, getErr := svc.GetCluster(context.Background(), "cluster-cap-02"); getErr == nil {
		t.Fatal("expected cluster-cap-02 not to be provisioned after reservation rejection")
	}

	_, ipErr := resAdapter.Reserve(context.Background(), contracts.ReservationRequest{
		TenantID:         "tenant-other",
		ResourceKind:     "CLUSTER",
		RawName:          "cluster-ip-full",
		QuotaUnits:       1,
		IPCIDRBlocks:     1,
		ClusterIPUtilPct: 85,
	})
	if !errors.IsConflictError(ipErr) {
		t.Fatalf("expected conflict error when cluster IP util exceeds 80%%, got: %v", ipErr)
	}
}

func TestSelfService_INV09_INV10_EC09_ApprovalAndPlanPinning(t *testing.T) {
	appAdapter := adapters.NewMemoryProvisioningApprovalAdapter()
	plan := contracts.PinnedPlan{
		TemplateSHA:    "tpl-sha-v1",
		PolicySHA:      "pol-sha-v1",
		Sizing:         "medium",
		NormalizedName: "cluster-pin-01",
	}
	planHash := rules.ComputePlanHash(plan)

	selfErr := appAdapter.RecordApproval(context.Background(), contracts.ApprovalRecord{
		ID:          "app-self-01",
		TenantID:    "tenant-pin",
		RequesterID: "alice",
		ApproverID:  "alice",
		PlanHash:    planHash,
		TemplateSHA: plan.TemplateSHA,
		PolicySHA:   plan.PolicySHA,
		BudgetUnits: 1,
		ExpiresAt:   time.Now().Add(time.Hour),
	})
	if selfErr == nil {
		t.Fatal("expected self-approval to be rejected")
	}

	if err := appAdapter.RecordApproval(context.Background(), contracts.ApprovalRecord{
		ID:          "app-expired-01",
		TenantID:    "tenant-pin",
		RequesterID: "alice",
		ApproverID:  "bob",
		PlanHash:    planHash,
		TemplateSHA: plan.TemplateSHA,
		PolicySHA:   plan.PolicySHA,
		BudgetUnits: 1,
		ExpiresAt:   time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("unexpected RecordApproval error: %v", err)
	}
	if expErr := appAdapter.VerifyAndConsumeApproval(context.Background(), "app-expired-01", plan, 10); !errors.IsConflictError(expErr) {
		t.Fatalf("expected conflict error for expired approval, got: %v", expErr)
	}

	if err := appAdapter.RecordApproval(context.Background(), contracts.ApprovalRecord{
		ID:          "app-valid-01",
		TenantID:    "tenant-pin",
		RequesterID: "alice",
		ApproverID:  "bob",
		PlanHash:    planHash,
		TemplateSHA: plan.TemplateSHA,
		PolicySHA:   plan.PolicySHA,
		BudgetUnits: 2,
		ExpiresAt:   time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("unexpected RecordApproval error: %v", err)
	}

	resCfg := resilience.DefaultResilienceConfig()
	clAdapter := adapters.NewGKEClusterClientAdapter(cluster.NewGKEClusterClient(resCfg))
	crAdapter := adapters.NewCloudResourceClientAdapter(cloudresource.NewCloudResourceClient(resCfg))
	svc := service.NewSelfServiceProvisioningService(clAdapter, crAdapter)
	svc.SetApprovalPort(appAdapter)

	driftedPlan := plan
	driftedPlan.TemplateSHA = "tpl-sha-v2"
	driftedCtx := service.WithApprovalPin(context.Background(), "app-valid-01", driftedPlan, 5)
	if _, driftErr := svc.ProvisionCluster(driftedCtx, createTestClusterSpec("cluster-pin-01")); !errors.IsConflictError(driftErr) {
		t.Fatalf("expected conflict error when template SHA drifts mid-saga, got: %v", driftErr)
	}

	overBudgetCtx := service.WithApprovalPin(context.Background(), "app-valid-01", plan, 1)
	if _, budgetErr := svc.ProvisionCluster(overBudgetCtx, createTestClusterSpec("cluster-pin-01")); !errors.IsConflictError(budgetErr) {
		t.Fatalf("expected conflict error when aggregate period budget exceeded, got: %v", budgetErr)
	}

	validCtx := service.WithApprovalPin(context.Background(), "app-valid-01", plan, 5)
	if _, okErr := svc.ProvisionCluster(validCtx, createTestClusterSpec("cluster-pin-01")); okErr != nil {
		t.Fatalf("expected valid pinned plan to provision, got: %v", okErr)
	}
}
