package tests

import (
	"strings"
	"testing"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/features/role-based-access-control/domain/rules"
)

func TestRoleBasedAccessControlRules_AuthorizationDecisionTable(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		subject        rbacContracts.Subject
		targetTenant   string
		resource       rbacContracts.Resource
		action         rbacContracts.Action
		expectedGrant  bool
		expectedReason string
	}{
		{
			name: "AdminGlobalAccess",
			subject: rbacContracts.Subject{
				ID:        "admin-1",
				TenantID:  "system-tenant",
				Roles:     []rbacContracts.RoleName{rbacContracts.RolePlatformAdmin},
				ExpiresAt: now.Add(1 * time.Hour),
			},
			targetTenant:   "client-tenant",
			resource:       rbacContracts.ResourceServiceCatalog,
			action:         rbacContracts.ActionDelete,
			expectedGrant:  true,
			expectedReason: "permission granted",
		},
		{
			name: "DeveloperValidSameTenant",
			subject: rbacContracts.Subject{
				ID:        "dev-1",
				TenantID:  "tenant-a",
				Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
				ExpiresAt: now.Add(1 * time.Hour),
			},
			targetTenant:   "tenant-a",
			resource:       rbacContracts.ResourceServiceCatalog,
			action:         rbacContracts.ActionWrite,
			expectedGrant:  true,
			expectedReason: "permission granted",
		},
		{
			name: "DeveloperCrossTenantViolation",
			subject: rbacContracts.Subject{
				ID:        "dev-1",
				TenantID:  "tenant-a",
				Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
				ExpiresAt: now.Add(1 * time.Hour),
			},
			targetTenant:   "tenant-b",
			resource:       rbacContracts.ResourceServiceCatalog,
			action:         rbacContracts.ActionWrite,
			expectedGrant:  false,
			expectedReason: "tenant isolation violation",
		},
		{
			name: "ViewerDeniedWrite",
			subject: rbacContracts.Subject{
				ID:        "viewer-1",
				TenantID:  "tenant-a",
				Roles:     []rbacContracts.RoleName{rbacContracts.RoleViewer},
				ExpiresAt: now.Add(1 * time.Hour),
			},
			targetTenant:   "tenant-a",
			resource:       rbacContracts.ResourceServiceCatalog,
			action:         rbacContracts.ActionWrite,
			expectedGrant:  false,
			expectedReason: "insufficient permissions",
		},
		{
			name: "MissingSubjectID",
			subject: rbacContracts.Subject{
				ExpiresAt: now.Add(1 * time.Hour),
			},
			targetTenant:   "tenant-a",
			resource:       rbacContracts.ResourceServiceCatalog,
			action:         rbacContracts.ActionRead,
			expectedGrant:  false,
			expectedReason: "missing subject identity",
		},
		{
			name: "ExpiredToken",
			subject: rbacContracts.Subject{
				ID:        "dev-expired",
				TenantID:  "tenant-a",
				Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
				ExpiresAt: now.Add(-10 * time.Minute),
			},
			targetTenant:   "tenant-a",
			resource:       rbacContracts.ResourceServiceCatalog,
			action:         rbacContracts.ActionRead,
			expectedGrant:  false,
			expectedReason: "token expired",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := rbacContracts.EvaluationRequest{
				Subject:        tc.subject,
				TargetTenantID: tc.targetTenant,
				Resource:       tc.resource,
				Action:         tc.action,
			}
			result := rules.EvaluateAuthorization(req, now)

			if result.Granted != tc.expectedGrant {
				t.Fatalf("expected Granted=%v, got Granted=%v (%s)", tc.expectedGrant, result.Granted, result.Reason)
			}
			if !strings.Contains(strings.ToLower(result.Reason), strings.ToLower(tc.expectedReason)) {
				t.Fatalf("expected reason %q, got %q", tc.expectedReason, result.Reason)
			}
		})
	}
}

func TestValidateJITRequest_SecurityCeilingAndApprover(t *testing.T) {
	validReq := rbacContracts.AccessGrantRequest{
		ID:              "grant-001",
		TenantID:        "tenant-finance",
		SubjectID:       "usr-alice",
		TargetNamespace: "payments-prod",
		Role:            "DeveloperDebug",
		Reason:          "investigating transaction deadlock incident INC-9982",
		DurationSeconds: 7200,
		ApproverRole:    "PlatformAdmin",
		IdempotencyKey:  "idemp-jit-001",
	}

	if err := rules.ValidateJITRequest(validReq); err != nil {
		t.Fatalf("expected valid JIT request to pass, got: %v", err)
	}

	ceilingExceededReq := validReq
	ceilingExceededReq.DurationSeconds = 18000
	if err := rules.ValidateJITRequest(ceilingExceededReq); err == nil {
		t.Fatalf("expected error when duration > 14400s ceiling, got nil")
	}

	invalidDurationReq := validReq
	invalidDurationReq.DurationSeconds = 0
	if err := rules.ValidateJITRequest(invalidDurationReq); err == nil {
		t.Fatalf("expected error when duration <= 0, got nil")
	}

	nodesProxyReq := validReq
	nodesProxyReq.Role = "nodes/proxy-viewer"
	if err := rules.ValidateJITRequest(nodesProxyReq); err == nil {
		t.Fatalf("expected V-25 violation when role contains nodes/proxy, got nil")
	}

	shortReasonReq := validReq
	shortReasonReq.Reason = "fix bug"
	if err := rules.ValidateJITRequest(shortReasonReq); err == nil {
		t.Fatalf("expected error when reason < 10 chars, got nil")
	}

	unauthorizedApproverReq := validReq
	unauthorizedApproverReq.ApproverRole = "Developer"
	if err := rules.ValidateJITRequest(unauthorizedApproverReq); err == nil {
		t.Fatalf("expected error when approverRole is not PlatformAdmin or SecurityLead, got nil")
	}
}

func TestValidateRoleSecurityPosture_RejectsNodesProxy(t *testing.T) {
	safePerms := []rbacContracts.Permission{
		{Resource: "pods", Action: "get"},
		{Resource: "services", Action: "list"},
	}
	if err := rules.ValidateRoleSecurityPosture("SafeRole", safePerms); err != nil {
		t.Fatalf("expected safe role to pass, got: %v", err)
	}

	proxyPerms := []rbacContracts.Permission{
		{Resource: "nodes/proxy", Action: "get"},
	}
	if err := rules.ValidateRoleSecurityPosture("DangerousRole", proxyPerms); err == nil {
		t.Fatalf("expected V-25 violation for nodes/proxy, got nil")
	}

	execPerms := []rbacContracts.Permission{
		{Resource: "nodes", Action: "nodes/exec"},
	}
	if err := rules.ValidateRoleSecurityPosture("DangerousRole", execPerms); err == nil {
		t.Fatalf("expected V-25 violation for nodes/exec, got nil")
	}
}
