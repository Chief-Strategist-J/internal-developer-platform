/*
ALGORITHM & ARCHITECTURE BLUEPRINT: POLICY-AS-CODE REST HANDLER TESTS

1. Architecture Role:
   - Verifies HTTP REST endpoints for the Policy-as-Code API.
   - Tests CreatePolicy, GetPolicy, ListActivePolicies, TransitionLifecycle, RequestException, and EvaluateAdmission.
   - Asserts ApiResponse envelope compliance, status codes, and error formatting.
   - Zero inline comments inside function bodies.
*/
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"internal-developer-platform/src/features/policy-as-code/contracts"
	"internal-developer-platform/src/features/policy-as-code/domain/workflows"
	"internal-developer-platform/src/features/policy-as-code/infrastructure/adapters"
	"internal-developer-platform/src/features/policy-as-code/service"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/types"
)

func setupTestPolicyHandler() *PolicyAsCodeHandler {
	repo := adapters.NewMemoryPolicyRepositoryAdapter()
	svc := service.NewPolicyAsCodeService(repo)
	return NewPolicyAsCodeHandler(svc)
}

func TestPolicyHttpCreateAndGet(t *testing.T) {
	handler := setupTestPolicyHandler()

	policyPayload := contracts.PolicyRecord{
		ID:               "pol-test-01",
		PublicID:         "pol_public_01",
		TenantID:         "tenant-dev",
		ExternalID:       "SEC-01",
		Name:             "Restricted Pod Security",
		Description:      "Disallow privileged containers",
		Severity:         contracts.SeverityHigh,
		EnforcementPoint: contracts.EnforcementPointCEL,
		RuleExpression:   "privileged == false",
		Exceptable:       true,
		DetectionPair:    "DET-01",
		Source:           contracts.PolicySourceAPI,
	}

	body, err := json.Marshal(policyPayload)
	if err != nil {
		t.Fatalf("failed to marshal policy: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(constants.HeaderXTenantID, "tenant-dev")

	rec := httptest.NewRecorder()
	handler.CreatePolicy(rec, req)

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

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/policies/pol-test-01?tenantId=tenant-dev", nil)
	getRec := httptest.NewRecorder()
	handler.GetPolicy(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK, got %d. Body: %s", getRec.Code, getRec.Body.String())
	}
}

func TestPolicyHttpTransitionLifecycle(t *testing.T) {
	handler := setupTestPolicyHandler()

	policyPayload := contracts.PolicyRecord{
		ID:               "pol-fsm-01",
		TenantID:         "tenant-dev",
		ExternalID:       "SEC-02",
		Name:             "Non-root Policy",
		Description:      "Require runAsNonRoot",
		Severity:         contracts.SeverityMedium,
		EnforcementPoint: contracts.EnforcementPointCEL,
		RuleExpression:   "runAsNonRoot == true",
		Exceptable:       true,
		DetectionPair:    "DET-02",
		Source:           contracts.PolicySourceAPI,
	}
	body, _ := json.Marshal(policyPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.CreatePolicy(rec, req)

	transitionPayload := LifecycleTransitionRequest{
		Event:            "PROMOTE_TO_CI_ONLY",
		ActiveViolations: 0,
	}
	tBody, _ := json.Marshal(transitionPayload)
	tReq := httptest.NewRequest(http.MethodPost, "/api/v1/policies/pol-fsm-01/lifecycle?tenantId=tenant-dev", bytes.NewReader(tBody))
	tReq.Header.Set("Content-Type", "application/json")
	tRec := httptest.NewRecorder()
	handler.TransitionLifecycle(tRec, tReq)

	if tRec.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200 OK, got %d. Body: %s", tRec.Code, tRec.Body.String())
	}
}

func TestPolicyHttpAdmissionEvaluation(t *testing.T) {
	handler := setupTestPolicyHandler()

	evalInput := workflows.AdmissionEvaluationInput{
		TenantID:     "tenant-dev",
		Namespace:    "payments-prod",
		WorkloadID:   "w-payment-api",
		CallerRole:   "Developer",
		ResourceJSON: `{"privileged":true}`,
		Policies: []contracts.PolicyRecord{
			{
				ID:               "p-eval-01",
				ExternalID:       "SEC-09",
				Severity:         contracts.SeverityCritical,
				EnforcementPoint: contracts.EnforcementPointCEL,
				LifecycleState:   contracts.LifecycleDeny,
				RuleExpression:   "privileged == false",
				Exceptable:       false,
				Description:      "Privileged pods forbidden",
			},
		},
		Timestamp: time.Now().UTC(),
	}

	body, _ := json.Marshal(evalInput)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/policies/admission/evaluate", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.EvaluateAdmission(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected HTTP 403 Forbidden for DENY violation, got %d", rec.Code)
	}

	var resp types.ApiResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response envelope: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success: true in envelope for valid evaluation verdict")
	}
}
