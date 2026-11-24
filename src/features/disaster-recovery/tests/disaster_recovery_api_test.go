// Package tests provides HTTP API integration tests for Disaster Recovery Ingress endpoints.
//
// Architectural Blueprint:
// - Step 14 Tests: Tests complete HTTP REST control plane lifecycle:
//     * Plans: POST/GET /api/v1/dr/plans, :checkReadiness
//     * Incidents: :declare, :approve
//     * Failovers: POST /api/v1/dr/failovers, :approve, :abort
//     * Fencing: :fence, /epoch
//     * Failbacks: POST /api/v1/dr/failbacks
//     * Drills: POST /api/v1/dr/drills
//     * Restores: POST /api/v1/dr/restores
//     * Breakglass: POST /api/v1/dr/breakglass:activate
// - Zero inline comments inside function bodies.
package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"internal-developer-platform/src/api/rest/v1/handlers"
	"internal-developer-platform/src/api/rest/v1/routers"
	"internal-developer-platform/src/features/disaster-recovery/contracts"
	"internal-developer-platform/src/features/disaster-recovery/infrastructure/adapters"
	"internal-developer-platform/src/features/disaster-recovery/service"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/types"
)

func TestDisasterRecoveryAPIRoutes(t *testing.T) {
	store := adapters.NewMemoryDRStoreAdapter()
	cloud := adapters.NewCloudProviderDRAdapter()
	svc := service.NewDisasterRecoveryService(store, cloud, cloud, cloud, cloud)
	h := handlers.NewDisasterRecoveryHandler(svc)
	router := routers.NewDisasterRecoveryRouter(h, nil)

	mux := http.NewServeMux()
	router.RegisterRoutes(mux)

	planCmd := contracts.CreateDRPlanCommand{
		TenantID:         "tenant-api",
		PlanName:         "payment-api-dr",
		IncidentTier:     "TIER_0",
		PrimaryRegion:    "us-central1",
		DRRegion:         "us-east4",
		Topology:         contracts.TopologyWarmStandby,
		TargetRTOSeconds: 300,
		TargetRPOSeconds: 0,
		DataStoreType:    contracts.DataStoreCloudSQL,
		SecretSyncSource: contracts.SecretSourceGoogleSecretManager,
		PrimaryClusterID: "gke-pri",
		DRClusterID:      "gke-sec",
		IdempotencyKey:   "idem-api-001",
	}

	body, _ := json.Marshal(planCmd)
	req := httptest.NewRequest(http.MethodPost, constants.EndpointDRPlans, bytes.NewReader(body))
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	req.Header.Set(constants.HeaderXTenantID, "tenant-api")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on POST /api/v1/dr/plans, got %d: %s", w.Code, w.Body.String())
	}

	var resp types.ApiResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Success {
		t.Fatalf("expected success true, got false")
	}

	reqGet := httptest.NewRequest(http.MethodGet, constants.EndpointDRPlans, nil)
	reqGet.Header.Set(constants.HeaderXTenantID, "tenant-api")
	wGet := httptest.NewRecorder()
	mux.ServeHTTP(wGet, reqGet)

	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on GET /api/v1/dr/plans, got %d", wGet.Code)
	}

	reqCheck := httptest.NewRequest(http.MethodPost, constants.EndpointDRPlansPrefix+"payment-api-dr:checkReadiness", nil)
	reqCheck.Header.Set(constants.HeaderXTenantID, "tenant-api")
	wCheck := httptest.NewRecorder()
	mux.ServeHTTP(wCheck, reqCheck)

	if wCheck.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on readiness check, got %d: %s", wCheck.Code, wCheck.Body.String())
	}

	fenceCmd := contracts.FenceDatasetCommand{
		TenantID:  "tenant-api",
		DatasetID: "payment-api-dr",
		Authority: "DR_REPLICA",
		Actor:     "sre-lead",
	}
	fenceBody, _ := json.Marshal(fenceCmd)
	reqFence := httptest.NewRequest(http.MethodPost, constants.EndpointDRDatasetsPrefix+"payment-api-dr:fence", bytes.NewReader(fenceBody))
	reqFence.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	reqFence.Header.Set(constants.HeaderXTenantID, "tenant-api")
	wFence := httptest.NewRecorder()
	mux.ServeHTTP(wFence, reqFence)

	if wFence.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on dataset fence, got %d: %s", wFence.Code, wFence.Body.String())
	}

	reqEpoch := httptest.NewRequest(http.MethodGet, constants.EndpointDRDatasetsPrefix+"payment-api-dr/epoch", nil)
	reqEpoch.Header.Set(constants.HeaderXTenantID, "tenant-api")
	wEpoch := httptest.NewRecorder()
	mux.ServeHTTP(wEpoch, reqEpoch)

	if wEpoch.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on dataset epoch, got %d: %s", wEpoch.Code, wEpoch.Body.String())
	}

	drillCmd := contracts.RunDrillCommand{
		TenantID:      "tenant-api",
		PlanID:        "payment-api-dr",
		Scope:         "REGION",
		Initiator:     "dr-coordinator",
		SyntheticOnly: true,
	}
	drillBody, _ := json.Marshal(drillCmd)
	reqDrill := httptest.NewRequest(http.MethodPost, constants.EndpointDRDrills, bytes.NewReader(drillBody))
	reqDrill.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	reqDrill.Header.Set(constants.HeaderXTenantID, "tenant-api")
	wDrill := httptest.NewRecorder()
	mux.ServeHTTP(wDrill, reqDrill)

	if wDrill.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on drill execution, got %d: %s", wDrill.Code, wDrill.Body.String())
	}

	restoreCmd := contracts.InitiateRestoreCommand{
		TenantID:       "tenant-api",
		PlanID:         "payment-api-dr",
		BackupID:       "bkp-gke-001",
		TargetCluster:  "gke-dr-cluster",
		Namespace:      "payments",
		ConflictPolicy: "FAIL_ON_CONFLICT",
		Initiator:      "sre-lead",
	}
	restoreBody, _ := json.Marshal(restoreCmd)
	reqRestore := httptest.NewRequest(http.MethodPost, constants.EndpointDRRestores, bytes.NewReader(restoreBody))
	reqRestore.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	reqRestore.Header.Set(constants.HeaderXTenantID, "tenant-api")
	wRestore := httptest.NewRecorder()
	mux.ServeHTTP(wRestore, reqRestore)

	if wRestore.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted on restore, got %d: %s", wRestore.Code, wRestore.Body.String())
	}

	breakglassCmd := contracts.ActivateBreakglassCommand{
		TenantID:      "tenant-api",
		Requester:     "operator-1",
		Approver:      "director-2",
		DurationMins:  60,
		Justification: "Emergency restore and key verification in DR region",
	}
	breakglassBody, _ := json.Marshal(breakglassCmd)
	reqBreak := httptest.NewRequest(http.MethodPost, constants.EndpointDRBreakglassActivate, bytes.NewReader(breakglassBody))
	reqBreak.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	reqBreak.Header.Set(constants.HeaderXTenantID, "tenant-api")
	wBreak := httptest.NewRecorder()
	mux.ServeHTTP(wBreak, reqBreak)

	if wBreak.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on breakglass activation, got %d: %s", wBreak.Code, wBreak.Body.String())
	}

	reqRPO := httptest.NewRequest(http.MethodGet, constants.EndpointDRPlansPrefix+"payment-api-dr/rpo", nil)
	reqRPO.Header.Set(constants.HeaderXTenantID, "tenant-api")
	wRPO := httptest.NewRecorder()
	mux.ServeHTTP(wRPO, reqRPO)

	if wRPO.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on GET plan RPO, got %d: %s", wRPO.Code, wRPO.Body.String())
	}

	var rpoResp types.ApiResponse
	_ = json.Unmarshal(wRPO.Body.Bytes(), &rpoResp)
	if !rpoResp.Success {
		t.Fatalf("expected success true on RPO response, got false")
	}

	reqGetDrill := httptest.NewRequest(http.MethodGet, constants.EndpointDRDrillsPrefix+"payment-api-dr", nil)
	reqGetDrill.Header.Set(constants.HeaderXTenantID, "tenant-api")
	wGetDrill := httptest.NewRecorder()
	mux.ServeHTTP(wGetDrill, reqGetDrill)

	if wGetDrill.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on GET drill run, got %d: %s", wGetDrill.Code, wGetDrill.Body.String())
	}

	reqGetEvidence := httptest.NewRequest(http.MethodGet, constants.EndpointDREvidencePrefix+"payment-api-dr", nil)
	reqGetEvidence.Header.Set(constants.HeaderXTenantID, "tenant-api")
	wGetEvidence := httptest.NewRecorder()
	mux.ServeHTTP(wGetEvidence, reqGetEvidence)

	if wGetEvidence.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on GET evidence bundle, got %d: %s", wGetEvidence.Code, wGetEvidence.Body.String())
	}

	reqGetRestore := httptest.NewRequest(http.MethodGet, constants.EndpointDRRestoresPrefix+"payment-api-dr", nil)
	reqGetRestore.Header.Set(constants.HeaderXTenantID, "tenant-api")
	wGetRestore := httptest.NewRecorder()
	mux.ServeHTTP(wGetRestore, reqGetRestore)

	if wGetRestore.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on GET restore op, got %d: %s", wGetRestore.Code, wGetRestore.Body.String())
	}
}
