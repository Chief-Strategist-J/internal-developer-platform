// Package bridges verifies Layer-3 cross-feature bridges via root index interfaces only (X-03, GO-INT-001).
package bridges_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"internal-developer-platform/src/app/bridges"
	gitopsreconciliation "internal-developer-platform/src/features/gitops-reconciliation"
	kubernetesworkloadprovisioning "internal-developer-platform/src/features/kubernetes-workload-provisioning"
	platformregime "internal-developer-platform/src/features/platform-regime"
	policyascode "internal-developer-platform/src/features/policy-as-code"
	rbac "internal-developer-platform/src/features/role-based-access-control"
)

type stubRBACBridgeProvider struct{}

func (s *stubRBACBridgeProvider) EvaluateTaskLease(_ context.Context, req rbac.TaskLeaseEvalRequest) (rbac.TaskLeaseEvalResult, error) {
	if req.TokenExpired || req.LeaseExpired || !req.HasSignedTaskLease {
		return rbac.TaskLeaseEvalResult{Granted: false, Reason: "expired or unsigned lease", Directive: "DENY"}, fmt.Errorf("unauthorized")
	}
	return rbac.TaskLeaseEvalResult{Granted: true, Directive: "PROCEED"}, nil
}

func (s *stubRBACBridgeProvider) EvaluateAsyncJobDrift(_ context.Context, _ string, _ string, queueLatency time.Duration) (rbac.AsyncJobDriftEvalResult, error) {
	if queueLatency > 15*time.Minute {
		return rbac.AsyncJobDriftEvalResult{Granted: false, Reason: "stale async job", Directive: "DISCARD"}, fmt.Errorf("revoked")
	}
	return rbac.AsyncJobDriftEvalResult{Granted: true, Directive: "PROCEED"}, nil
}

type stubRegimeBridgeProvider struct{}

func (s *stubRegimeBridgeProvider) EvaluateWorkloadCompliance(_ context.Context, _ string, input platformregime.WorkloadComplianceEvalInput) (bool, []platformregime.WorkloadComplianceEvalViolation, error) {
	if input.AuditRetentionDays < 90 || (input.HasEncryptedSecrets && input.KMSKeyURI == "") {
		return false, []platformregime.WorkloadComplianceEvalViolation{{Rule: "REGIME-RETENTION-KMS", Message: "insufficient retention or missing KMS key"}}, nil
	}
	return true, nil, nil
}

type stubPolicyBridgeProvider struct{}

func (s *stubPolicyBridgeProvider) GetActiveExceptionForWorkload(_ context.Context, tenantID string, namespace string, workloadName string) (*policyascode.ActiveWorkloadException, error) {
	if tenantID == "tenant-1" && namespace == "ns-1" && workloadName == "api-worker" {
		return &policyascode.ActiveWorkloadException{PublicID: "EXC-BRIDGE-01", ApproverID: "sec-lead"}, nil
	}
	return nil, nil
}

func TestGitopsRBACBridge_EvaluatesTaskLease(t *testing.T) {
	var port gitopsreconciliation.TaskLeaseBridge = bridges.NewGitopsRBACBridge(&stubRBACBridgeProvider{})

	res, err := port.EvaluateTaskLease(context.Background(), gitopsreconciliation.TaskLeaseRequest{
		SubjectID:    "runner-1",
		TenantID:     "tenant-1",
		LeaseToken:   "signed-lease-tok",
		TokenExpired: false,
		LeaseExpired: false,
	})
	if err != nil || !res.Granted {
		t.Fatalf("expected valid lease to grant, got res=%+v err=%v", res, err)
	}

	res, err = port.EvaluateTaskLease(context.Background(), gitopsreconciliation.TaskLeaseRequest{
		SubjectID:    "runner-1",
		TenantID:     "tenant-1",
		LeaseToken:   "signed-lease-tok",
		TokenExpired: true,
		LeaseExpired: true,
	})
	if err == nil || res.Granted {
		t.Fatalf("expected expired lease + expired token to deny, got res=%+v err=%v", res, err)
	}
}

func TestWorkloadRBACBridge_EvaluatesAsyncJobDrift(t *testing.T) {
	var port kubernetesworkloadprovisioning.AsyncDriftBridge = bridges.NewWorkloadRBACBridge(&stubRBACBridgeProvider{})

	res, err := port.EvaluateAsyncJobDrift(context.Background(), kubernetesworkloadprovisioning.AsyncJobDriftRequest{
		SubjectID:    "worker-1",
		JTI:          "jti-ok",
		QueueLatency: 5 * time.Second,
	})
	if err != nil || res.Revoked || !res.Granted {
		t.Fatalf("expected short-latency async job to pass, got res=%+v err=%v", res, err)
	}

	res, err = port.EvaluateAsyncJobDrift(context.Background(), kubernetesworkloadprovisioning.AsyncJobDriftRequest{
		SubjectID:    "worker-1",
		JTI:          "jti-stale",
		QueueLatency: 2 * time.Hour,
	})
	if err == nil || !res.Revoked || res.Granted {
		t.Fatalf("expected stale async job to be revoked, got res=%+v err=%v", res, err)
	}
}

func TestWorkloadRegimeBridge_EvaluatesCompliance(t *testing.T) {
	var guard kubernetesworkloadprovisioning.RegimeGuardBridge = bridges.NewWorkloadRegimeBridge(&stubRegimeBridgeProvider{})

	res, err := guard.EvaluateWorkloadRegime(context.Background(), kubernetesworkloadprovisioning.RegimeEvaluationRequest{
		TenantID:            "tenant-1",
		Namespace:           "fintech-prod",
		AuditRetentionDays:  30,
		HasEncryptedSecrets: true,
		KMSKeyURI:           "",
	})
	if err != nil || res.Allowed || len(res.Violations) == 0 {
		t.Fatalf("expected regime violation for low retention and missing KMS key, got res=%+v err=%v", res, err)
	}
}

func TestWorkloadPolicyBridge_FetchesActiveException(t *testing.T) {
	var guard kubernetesworkloadprovisioning.PolicyExceptionBridge = bridges.NewWorkloadPolicyBridge(&stubPolicyBridgeProvider{})

	exc, err := guard.GetActiveExceptionForWorkload(context.Background(), "tenant-1", "ns-1", "api-worker")
	if err != nil || exc == nil || exc.PublicID != "EXC-BRIDGE-01" {
		t.Fatalf("expected active exception EXC-BRIDGE-01 via bridge, got exc=%+v err=%v", exc, err)
	}
}

type stubDRBridgeProvider struct {
	fencedWorkloads map[string]bool
}

func (s *stubDRBridgeProvider) IsWorkloadFenced(_ context.Context, _ string, workloadID string) (bool, error) {
	return s.fencedWorkloads[workloadID], nil
}

func (s *stubDRBridgeProvider) GetFencingEpoch(_ context.Context, _ string) (int64, error) {
	return 1, nil
}

func TestDeploymentDRBridge_EvaluatesFencing(t *testing.T) {
	provider := &stubDRBridgeProvider{
		fencedWorkloads: map[string]bool{
			"payments-service": true,
			"catalog-service":  false,
		},
	}
	bridge := bridges.NewDeploymentDRBridge(provider)

	fenced, err := bridge.IsWorkloadFenced(context.Background(), "tenant-1", "payments-service")
	if err != nil || !fenced {
		t.Fatalf("expected payments-service to be fenced, got fenced=%v err=%v", fenced, err)
	}

	notFenced, err := bridge.IsWorkloadFenced(context.Background(), "tenant-1", "catalog-service")
	if err != nil || notFenced {
		t.Fatalf("expected catalog-service to NOT be fenced, got notFenced=%v err=%v", notFenced, err)
	}
}
