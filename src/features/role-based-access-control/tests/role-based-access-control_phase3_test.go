package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/features/role-based-access-control/domain/rules"
	"internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
)

func TestEvaluateSidecarPDPFallback(t *testing.T) {
	now := time.Now().UTC()
	req := rbacContracts.EvaluationRequest{
		Subject: rbacContracts.Subject{
			ID:       "user-1",
			TenantID: "tenant-a",
		},
		TargetTenantID: "tenant-a",
		Resource:       "deployments",
		Action:         rbacContracts.ActionRead,
	}

	allowedRes := &rbacContracts.SidecarDecisionResult{
		Allowed: true,
		Engine:  rbacContracts.SidecarEngineOPA,
	}
	res := rules.EvaluateSidecarPDPFallback(allowedRes, req, now)
	if !res.Granted {
		t.Fatalf("expected sidecar allow to grant access, got %+v", res)
	}

	deniedRes := &rbacContracts.SidecarDecisionResult{
		Allowed:       false,
		Engine:        rbacContracts.SidecarEngineCedar,
		Reason:        "cedar policy denied",
		ViolationCode: "ERR_CEDAR_DENY",
	}
	res = rules.EvaluateSidecarPDPFallback(deniedRes, req, now)
	if res.Granted {
		t.Fatalf("expected sidecar deny to return denied, got %+v", res)
	}
}

func TestEvaluateEBPFHookEnforcement(t *testing.T) {
	policy := rbacContracts.EBPFHookPolicy{
		AllowedSyscalls: []string{"read", "write"},
		DeniedPaths:     []string{"/etc/shadow", "/var/run/secrets"},
		Enforce:         true,
	}

	if err := rules.EvaluateEBPFHookEnforcement(rbacContracts.EBPFHookEvent{
		Syscall:    "read",
		TargetPath: "/app/config.yaml",
	}, policy); err != nil {
		t.Fatalf("expected allowed syscall and path, got %v", err)
	}

	if err := rules.EvaluateEBPFHookEnforcement(rbacContracts.EBPFHookEvent{
		Syscall:    "read",
		TargetPath: "/var/run/secrets/token",
	}, policy); err == nil {
		t.Fatalf("expected denied path to fail")
	}

	if err := rules.EvaluateEBPFHookEnforcement(rbacContracts.EBPFHookEvent{
		Syscall:    "ptrace",
		TargetPath: "/app/config.yaml",
	}, policy); err == nil {
		t.Fatalf("expected disallowed syscall to fail")
	}
}

func TestAnalyzeLeastPrivilegeAndPrune(t *testing.T) {
	now := time.Now().UTC()
	roles := map[rbacContracts.RoleName][]rbacContracts.Permission{
		"dev": {
			{Resource: "deployments", Action: rbacContracts.ActionRead},
			{Resource: "secrets", Action: rbacContracts.ActionDelete},
		},
	}
	usage := []rbacContracts.PermissionUsageRecord{
		{
			Role:       "dev",
			Resource:   "deployments",
			Action:     rbacContracts.ActionRead,
			LastUsedAt: now.Add(-1 * time.Hour),
			HitCount:   42,
		},
	}

	report := rules.AnalyzeLeastPrivilege(roles, usage, 24*time.Hour, now)
	if len(report.Findings) != 1 || report.Findings[0].Permission.Resource != "secrets" {
		t.Fatalf("expected 1 unused permission finding for secrets, got %+v", report.Findings)
	}

	pruned := rules.PruneUnusedPermissions(roles["dev"], report.Findings)
	if len(pruned) != 1 || pruned[0].Resource != "deployments" {
		t.Fatalf("expected only deployments permission to remain, got %+v", pruned)
	}
}

func TestHTTPSidecarPDPAdapter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":true}`))
	}))
	defer srv.Close()

	adapter := adapters.NewHTTPSidecarPDPAdapter(rbacContracts.SidecarPDPConfig{
		Engine:    rbacContracts.SidecarEngineOPA,
		Endpoint:  srv.URL,
		TimeoutMs: 500,
	})

	if err := adapter.HealthCheck(context.Background()); err != nil {
		t.Fatalf("expected health check to succeed, got %v", err)
	}

	res, err := adapter.EvaluateSidecar(context.Background(), rbacContracts.SidecarDecisionRequest{
		SubjectID: "user-1",
		TenantID:  "tenant-a",
		Resource:  "deployments",
		Action:    rbacContracts.ActionRead,
	})
	if err != nil || !res.Allowed {
		t.Fatalf("expected sidecar allow, got res=%+v err=%v", res, err)
	}
}
