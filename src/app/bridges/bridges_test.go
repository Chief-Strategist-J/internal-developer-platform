// Package bridges verifies Layer-3 cross-feature bridge and capability wiring (X-02, X-03).
package bridges_test

import (
	"context"
	"testing"
	"time"

	"internal-developer-platform/src/app/bridges"
	gitopsPorts "internal-developer-platform/src/features/gitops-reconciliation/infrastructure/ports"
	workloadPorts "internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/ports"
	rbacAdapters "internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
	rbacService "internal-developer-platform/src/features/role-based-access-control/service"
	"internal-developer-platform/src/shared/kernel"
	"internal-developer-platform/src/shared/kernel/capabilities/authz"
)

func setupTestRBACService() *rbacService.RBACService {
	verifier := rbacAdapters.NewJWTTokenVerifierAdapter("bridge-test-secret-0123456789", "idp-test")
	revocation := rbacAdapters.NewMemoryRevocationAdapter()
	grantRepo := rbacAdapters.NewMemoryAccessGrantAdapter()
	return rbacService.NewRBACServiceWithDeps(rbacService.RBACServiceDeps{
		TokenVerifier:   verifier,
		RevocationStore: revocation,
		GrantRepo:       grantRepo,
		BootstrapSecret: "bridge-bootstrap",
		Environment:     "dev",
	})
}

func TestAuthorizerCapabilityProvider_CheckEnforcesRBAC(t *testing.T) {
	svc := setupTestRBACService()
	provider := rbacService.NewRBACAuthorizerProvider(svc)

	dec, portErr := provider.Check(context.Background(), authz.CheckRequest{
		Principal: kernel.Principal{
			Subject:  "dev-1",
			TenantID: "tenant-1",
			Roles:    []string{"Developer"},
		},
		TenantID: "tenant-1",
		Resource: string(authz.ResourceServiceCatalog),
		Action:   string(authz.ActionRead),
	})
	if portErr != nil || dec != authz.DecisionPermit {
		t.Fatalf("expected Developer read on service-catalog to PERMIT, got decision=%s err=%v", dec, portErr)
	}

	dec, portErr = provider.Check(context.Background(), authz.CheckRequest{
		Principal: kernel.Principal{
			Subject:  "viewer-1",
			TenantID: "tenant-1",
			Roles:    []string{"Viewer"},
		},
		TenantID: "tenant-2",
		Resource: string(authz.ResourceInfrastructure),
		Action:   string(authz.ActionWrite),
	})
	if portErr == nil || dec != authz.DecisionDeny {
		t.Fatalf("expected Viewer cross-tenant write to DENY, got decision=%s err=%v", dec, portErr)
	}
}

func TestGitopsRBACBridge_EvaluatesTaskLease(t *testing.T) {
	svc := setupTestRBACService()
	var port gitopsPorts.TaskLeaseGuardPort = bridges.NewGitopsRBACBridge(svc)

	res, err := port.EvaluateTaskLease(context.Background(), gitopsPorts.TaskLeaseRequest{
		SubjectID:    "runner-1",
		TenantID:     "tenant-1",
		LeaseToken:   "signed-lease-tok",
		TokenExpired: false,
		LeaseExpired: false,
	})
	if err != nil || !res.Granted {
		t.Fatalf("expected valid lease to grant, got res=%+v err=%v", res, err)
	}

	res, err = port.EvaluateTaskLease(context.Background(), gitopsPorts.TaskLeaseRequest{
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
	svc := setupTestRBACService()
	var port workloadPorts.AsyncJobDriftPort = bridges.NewWorkloadRBACBridge(svc)

	res, err := port.EvaluateAsyncJobDrift(context.Background(), workloadPorts.AsyncJobDriftRequest{
		SubjectID:    "worker-1",
		JTI:          "jti-ok",
		QueueLatency: 5 * time.Second,
	})
	if err != nil || res.Revoked || !res.Granted {
		t.Fatalf("expected short-latency async job to pass, got res=%+v err=%v", res, err)
	}

	res, err = port.EvaluateAsyncJobDrift(context.Background(), workloadPorts.AsyncJobDriftRequest{
		SubjectID:    "worker-1",
		JTI:          "jti-stale",
		QueueLatency: 2 * time.Hour,
	})
	if err == nil || !res.Revoked || res.Granted {
		t.Fatalf("expected stale async job without OIDC revalidation to be revoked, got res=%+v err=%v", res, err)
	}
}
