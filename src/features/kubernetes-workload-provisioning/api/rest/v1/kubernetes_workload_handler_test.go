/*
Package handlers tests verify the HTTP REST endpoints for the Kubernetes Workload Provisioning API.
It tests command validation, header extraction, response envelope compliance, and error formatting.
*/
package restv1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/adapters"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/service"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/types"
)

func setupTestHandler() *KubernetesWorkloadHandler {
	store := adapters.NewMemoryWorkloadRepositoryAdapter()
	k8sClient := adapters.NewKubernetesClientAdapter("https://10.0.0.1:6443")
	svc := service.NewWorkloadProvisioningService(store, k8sClient)
	return NewKubernetesWorkloadHandler(svc)
}

func TestProvisionWorkloadHttpSuccess(t *testing.T) {
	handler := setupTestHandler()

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-http-01",
		TenantID:       "tenant-dev",
		ServiceID:      "billing-svc",
		Namespace:      "billing-dev",
		WorkloadName:   "billing-api",
		Image:          "gcr.io/bank/billing:v1.0.0",
		CPURequest:     "250m",
		CPULimit:       "1000m",
		MemoryRequest:  "256Mi",
		MemoryLimit:    "1Gi",
		Replicas:       2,
		IdempotencyKey: "idemp-http-001",
	}

	body, err := json.Marshal(cmd)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workloads", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(constants.HeaderXTenantID, "tenant-dev")
	req.Header.Set(constants.HeaderXIdempotencyKey, "idemp-http-001")
	req.Header.Set(constants.HeaderXRequestID, "req-123")

	rec := httptest.NewRecorder()
	handler.ProvisionWorkload(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected HTTP 201 Created, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var resp types.ApiResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response envelope: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected success: true in envelope")
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected statusCode: 201 in envelope")
	}
}

func TestProvisionWorkloadHttpMalformedPayload(t *testing.T) {
	handler := setupTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workloads", bytes.NewReader([]byte("{invalid-json")))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	handler.ProvisionWorkload(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected HTTP 400 Bad Request, got %d", rec.Code)
	}

	var resp types.ApiResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode error envelope: %v", err)
	}

	if resp.Success {
		t.Errorf("expected success: false in error response")
	}
	if resp.Error == nil || resp.Error.Code != constants.ErrCodeValidationFailed {
		t.Errorf("expected validation failed error code")
	}
}

func TestGetWorkloadHttpSuccess(t *testing.T) {
	handler := setupTestHandler()

	cmd := contracts.WorkloadProvisionCommand{
		ID:             "wkld-get-01",
		TenantID:       "tenant-dev",
		ServiceID:      "search-svc",
		Namespace:      "search-dev",
		WorkloadName:   "search-api",
		Image:          "gcr.io/bank/search:v1.0.0",
		Replicas:       1,
		IdempotencyKey: "idemp-get-001",
	}

	body, _ := json.Marshal(cmd)
	provReq := httptest.NewRequest(http.MethodPost, "/api/v1/workloads", bytes.NewReader(body))
	provRec := httptest.NewRecorder()
	handler.ProvisionWorkload(provRec, provReq)

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/wkld-get-01", nil)
	getReq.Header.Set(constants.HeaderXTenantID, "tenant-dev")
	getRec := httptest.NewRecorder()
	handler.GetWorkload(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK, got %d", getRec.Code)
	}

	var resp types.ApiResponse
	if err := json.NewDecoder(getRec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected success: true")
	}
}
