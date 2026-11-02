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
