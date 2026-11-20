package rules

import (
	"strings"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/shared/errors"
)

const (
	MaxJITSessionDurationSeconds = 14400
	MinJITSessionDurationSeconds = 60
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
	if !isGlobalAdmin {
		if req.TargetTenantID != "" && req.Subject.TenantID != "" && req.Subject.TenantID != req.TargetTenantID {
			return rbacContracts.EvaluationResult{
				Granted: false,
				Reason:  "tenant isolation violation: subject tenant does not match target tenant",
			}
		}
	}
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

func ValidateJITRequest(req rbacContracts.AccessGrantRequest) error {
	if req.ID == "" || req.TenantID == "" || req.SubjectID == "" || req.TargetNamespace == "" {
		return errors.NewValidationError("id, tenantId, subjectId, and targetNamespace are required")
	}
	if req.Role == "" {
		return errors.NewValidationError("requested role is required")
	}
	roleLower := strings.ToLower(req.Role)
	if strings.Contains(roleLower, "nodes/proxy") || strings.Contains(roleLower, "nodes/exec") {
		return errors.NewForbiddenError("V-25 violation: nodes/proxy and kubelet exec permissions are strictly prohibited")
	}
	if req.DurationSeconds <= 0 || req.DurationSeconds > MaxJITSessionDurationSeconds {
		return errors.NewValidationError("durationSeconds exceeds security ceiling of 4 hours (14400 seconds)")
	}
	if len(strings.TrimSpace(req.Reason)) < 10 {
		return errors.NewValidationError("mandatory incident/business justification required (min 10 chars)")
	}
	if req.ApproverRole != "PlatformAdmin" && req.ApproverRole != "SecurityLead" {
		return errors.NewForbiddenError("JIT access grants must be approved by PlatformAdmin or SecurityLead")
	}
	return nil
}

func ValidateRoleSecurityPosture(roleName string, perms []rbacContracts.Permission) error {
	for _, p := range perms {
		res := strings.ToLower(string(p.Resource))
		act := strings.ToLower(string(p.Action))
		if strings.Contains(res, "nodes/proxy") || strings.Contains(act, "nodes/proxy") ||
			strings.Contains(res, "nodes/exec") || strings.Contains(act, "nodes/exec") {
			return errors.NewForbiddenError("V-25 violation: nodes/proxy is strictly prohibited for human and monitoring roles")
		}
	}
	return nil
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
