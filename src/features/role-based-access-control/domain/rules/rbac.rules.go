package rules

import (
	"strings"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
)

var DefaultRoleDefinitions = map[rbacContracts.RoleName][]rbacContracts.Permission{
	rbacContracts.RolePlatformAdmin: {
		{Resource: rbacContracts.ResourceAll, Action: rbacContracts.ActionAll},
	},
	rbacContracts.RoleDeveloper: {
		{Resource: rbacContracts.ResourceAuth, Action: rbacContracts.ActionRead},
		{Resource: rbacContracts.ResourceServiceCatalog, Action: rbacContracts.ActionRead},
		{Resource: rbacContracts.ResourceServiceCatalog, Action: rbacContracts.ActionWrite},
		{Resource: rbacContracts.ResourceServiceCatalog, Action: rbacContracts.ActionUpdate},
		{Resource: rbacContracts.ResourceInfrastructure, Action: rbacContracts.ActionRead},
	},
	rbacContracts.RoleViewer: {
		{Resource: rbacContracts.ResourceAuth, Action: rbacContracts.ActionRead},
		{Resource: rbacContracts.ResourceServiceCatalog, Action: rbacContracts.ActionRead},
		{Resource: rbacContracts.ResourceInfrastructure, Action: rbacContracts.ActionRead},
	},
}

// EvaluateAuthorization determines whether the subject is permitted to execute the action on the resource
func EvaluateAuthorization(req rbacContracts.EvaluationRequest, now time.Time) rbacContracts.EvaluationResult {
	if req.Subject.ID == "" {
		return rbacContracts.EvaluationResult{Granted: false, Reason: "missing subject identity"}
	}

	if !req.Subject.ExpiresAt.IsZero() && now.After(req.Subject.ExpiresAt) {
		return rbacContracts.EvaluationResult{Granted: false, Reason: "token expired"}
	}

	isGlobalAdmin := false
	for _, r := range req.Subject.Roles {
		if r == rbacContracts.RolePlatformAdmin {
			isGlobalAdmin = true
			break
		}
	}

	// Tenant boundary check: non-global-admins must match target tenant
	if !isGlobalAdmin {
		if req.TargetTenantID != "" && req.Subject.TenantID != "" && req.Subject.TenantID != req.TargetTenantID {
			return rbacContracts.EvaluationResult{
				Granted: false,
				Reason:  "tenant isolation violation: subject tenant does not match target tenant",
			}
		}
	}

	// Collect effective permissions
	var effectivePermissions []rbacContracts.Permission
	effectivePermissions = append(effectivePermissions, req.Subject.Permissions...)

	for _, roleName := range req.Subject.Roles {
		if perms, ok := DefaultRoleDefinitions[roleName]; ok {
			effectivePermissions = append(effectivePermissions, perms...)
		}
	}

	for _, perm := range effectivePermissions {
		if matchesPermission(perm, req.Resource, req.Action) {
			return rbacContracts.EvaluationResult{
				Granted: true,
				Reason:  "permission granted",
			}
		}
	}

	return rbacContracts.EvaluationResult{
		Granted: false,
		Reason:  "insufficient permissions for resource and action",
	}
}

func matchesPermission(perm rbacContracts.Permission, targetResource rbacContracts.Resource, targetAction rbacContracts.Action) bool {
	resourceMatch := perm.Resource == rbacContracts.ResourceAll ||
		perm.Resource == targetResource ||
		strings.EqualFold(string(perm.Resource), string(targetResource))

	if !resourceMatch {
		return false
	}

	actionMatch := perm.Action == rbacContracts.ActionAll ||
		perm.Action == targetAction ||
		strings.EqualFold(string(perm.Action), string(targetAction))

	return actionMatch
}
