package rules

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/shared/errors"
)

const (
	MaxJITSessionDurationSeconds   = 14400
	MinJITSessionDurationSeconds   = 60
	MaxBreakGlassTTLSeconds        = 7200
	MaxRoleHierarchyDepth          = 5
	MaxPermissibleQueueDrift       = 5 * time.Minute
	SplitBrainStaleGraceWindow     = 2 * time.Hour
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
		return rbacContracts.EvaluationResult{Granted: false, Reason: "missing subject identity", ViolationCode: "ERR_UNAUTHENTICATED"}
	}
	if !req.Subject.ExpiresAt.IsZero() && now.After(req.Subject.ExpiresAt) {
		return rbacContracts.EvaluationResult{Granted: false, Reason: "token expired", ViolationCode: "ERR_TOKEN_EXPIRED"}
	}
	if req.BreakGlass != nil {
		bgRes := EvaluateBreakGlassElevation(*req.BreakGlass)
		if !bgRes.Granted {
			return bgRes
		}
		return rbacContracts.EvaluationResult{
			Granted:        true,
			ElevatedPermit: true,
			Reason:         "break-glass elevated session active",
		}
	}
	isGlobalAdmin := false
	for _, r := range req.Subject.Roles {
		if r == rbacContracts.RolePlatformAdmin {
			isGlobalAdmin = true
			break
		}
	}
	if isGlobalAdmin && isRootGuardrailViolated(req.Resource, req.Action) {
		return rbacContracts.EvaluationResult{
			Granted:       false,
			Reason:        "guardrail violation prevents root action on sensitive resource",
			ViolationCode: "ERR_ROOT_GUARDRAIL",
		}
	}
	if !isGlobalAdmin {
		if req.TargetTenantID != "" && req.Subject.TenantID != "" && req.Subject.TenantID != req.TargetTenantID {
			if !VerifyCrossTenantFederationHandshake(req.CrossTenantFederation, req.Subject.TenantID, req.TargetTenantID, req.FederationHMACSecret, now) {
				return rbacContracts.EvaluationResult{
					Granted:       false,
					Reason:        "tenant isolation violation: subject tenant does not match target tenant",
					ViolationCode: "ERR_TENANT_ISOLATION",
				}
			}
		}
	}
	for _, deny := range req.Subject.DenyPermissions {
		if matchesPermissionExact(deny, req.Resource, req.Action) {
			return rbacContracts.EvaluationResult{
				Granted:       false,
				Reason:        "explicit deny rule overrides all grants",
				ViolationCode: "ERR_EXPLICIT_DENY",
			}
		}
	}
	roleDefs := req.ResolvedRolePermissions
	if len(roleDefs) == 0 {
		roleDefs = DefaultRoleDefinitions
	}
	var effectivePermissions []rbacContracts.Permission
	effectivePermissions = append(effectivePermissions, req.Subject.Permissions...)
	for _, roleName := range req.Subject.Roles {
		if perms, ok := roleDefs[roleName]; ok {
			effectivePermissions = append(effectivePermissions, perms...)
		}
	}
	if len(effectivePermissions) == 0 {
		return rbacContracts.EvaluationResult{
			Granted:       false,
			Reason:        "no applicable policy found for subject",
			ViolationCode: "ERR_NO_APPLICABLE_POLICY",
		}
	}
	permitted := false
	if len(req.Subject.FlattenedPermissions) > 0 {
		permitted = hasPermissionInHashset(req.Subject.FlattenedPermissions, req.Resource, req.Action, isGlobalAdmin)
	}
	if !permitted {
		for _, perm := range effectivePermissions {
			if matchesPermission(perm, req.Resource, req.Action, isGlobalAdmin) {
				permitted = true
				break
			}
		}
	}
	if !permitted {
		return rbacContracts.EvaluationResult{
			Granted:       false,
			Reason:        "insufficient permissions for resource and action",
			ViolationCode: "ERR_INSUFFICIENT_PERMISSIONS",
		}
	}
	if abacRes := evaluateABACGuards(req, now); !abacRes.Granted {
		return abacRes
	}
	return rbacContracts.EvaluationResult{
		Granted: true,
		Reason:  "permission granted",
	}
}

func EvaluateBreakGlassElevation(bg rbacContracts.BreakGlassContext) rbacContracts.EvaluationResult {
	if bg.TicketID == "" || !bg.TicketApproved || bg.TTLSeconds <= 0 || bg.TTLSeconds > MaxBreakGlassTTLSeconds {
		return rbacContracts.EvaluationResult{
			Granted:       false,
			Reason:        "invalid or unapproved break-glass ticket or TTL exceeds 2h limit",
			ViolationCode: "ERR_BREAK_GLASS_INVALID",
		}
	}
	if bg.Tier0Prod && !bg.DualApprovalGranted {
		return rbacContracts.EvaluationResult{
			Granted:       false,
			Reason:        "tier-0 production break-glass requires dual authorization approval",
			ViolationCode: "ERR_DUAL_APPROVAL_REQUIRED",
		}
	}
	return rbacContracts.EvaluationResult{
		Granted:        true,
		ElevatedPermit: true,
		Reason:         "break-glass elevation approved",
	}
}

func ValidateRoleHierarchyDAG(edges map[string][]string) error {
	state := make(map[string]int)
	height := make(map[string]int)
	var dfs func(node string) (int, error)
	dfs = func(node string) (int, error) {
		if state[node] == 1 {
			return 0, errors.NewValidationError("cyclic dependency detected in role hierarchy")
		}
		if state[node] == 2 {
			return height[node], nil
		}
		state[node] = 1
		maxChild := 0
		for _, next := range edges[node] {
			h, err := dfs(next)
			if err != nil {
				return 0, err
			}
			if h > maxChild {
				maxChild = h
			}
		}
		state[node] = 2
		height[node] = 1 + maxChild
		if height[node] > MaxRoleHierarchyDepth {
			return 0, errors.NewValidationError("role hierarchy exceeds maximum nesting depth of 5")
		}
		return height[node], nil
	}
	for node := range edges {
		if state[node] == 0 {
			if _, err := dfs(node); err != nil {
				return err
			}
		}
	}
	return nil
}

func EvaluateImpersonation(callerMTLS bool, hasImpersonateGrant bool, targetProtectedAdmin bool) error {
	if !callerMTLS {
		return errors.NewForbiddenError("impersonation requires trusted mTLS workload certificate")
	}
	if !hasImpersonateGrant {
		return errors.NewForbiddenError("calling service lacks iam.impersonate grant")
	}
	if targetProtectedAdmin {
		return errors.NewForbiddenError("protected admin identities cannot be impersonated")
	}
	return nil
}

func EvaluateDelegationScope(isTenantDelegation bool, tokenTenantID string, targetTenantID string) error {
	if !isTenantDelegation {
		return errors.NewForbiddenError("confused deputy guard: shared service account invocation prohibited")
	}
	if tokenTenantID == "" || tokenTenantID != targetTenantID {
		return errors.NewForbiddenError("delegation token scope does not match target tenant")
	}
	return nil
}

func isRootGuardrailViolated(resource rbacContracts.Resource, action rbacContracts.Action) bool {
	if resource == rbacContracts.ResourceAuditLogs && action == rbacContracts.ActionDelete {
		return true
	}
	if resource == rbacContracts.ResourceCustomerPII && action == rbacContracts.ActionRead {
		return true
	}
	return false
}

func evaluateABACGuards(req rbacContracts.EvaluationRequest, now time.Time) rbacContracts.EvaluationResult {
	if req.AllowedEndHourUTC > req.AllowedStartHourUTC {
		hour := now.UTC().Hour()
		if hour < req.AllowedStartHourUTC || hour >= req.AllowedEndHourUTC {
			return rbacContracts.EvaluationResult{
				Granted:       false,
				Reason:        "attribute policy deny: outside allowed UTC time-of-day window",
				ViolationCode: "ERR_OUTSIDE_ALLOWED_HOURS",
			}
		}
	}
	if strings.EqualFold(req.TargetEnvironment, rbacContracts.EnvDevelopment) || strings.EqualFold(req.TargetEnvironment, rbacContracts.EnvStaging) {
		if req.MaxTenantQuota > 0 && req.CurrentQuotaUsage >= req.MaxTenantQuota {
			return rbacContracts.EvaluationResult{
				Granted:       false,
				Reason:        "tenant quota exceeded in dev/staging environment",
				ViolationCode: "ERR_TENANT_QUOTA_EXCEEDED",
			}
		}
	}
	if strings.EqualFold(req.TargetEnvironment, rbacContracts.EnvProduction) {
		if req.ChangeFreezeActive {
			return rbacContracts.EvaluationResult{
				Granted:       false,
				Reason:        "production deployment blocked: active change freeze in effect",
				ViolationCode: "ERR_CHANGE_FREEZE",
			}
		}
		if req.RequireStepUpMFA && !req.Subject.MFAVerified {
			return rbacContracts.EvaluationResult{
				Granted:       false,
				Reason:        "step-up WebAuthn/FIDO2 MFA required for production",
				ViolationCode: "ERR_STEP_UP_MFA_REQUIRED",
			}
		}
	}
	if req.ResourceClassification != "" && req.Subject.Classification != req.ResourceClassification {
		return rbacContracts.EvaluationResult{
			Granted:       false,
			Reason:        "attribute policy deny: resource data classification mismatch",
			ViolationCode: "ERR_CLASSIFICATION_MISMATCH",
		}
	}
	return rbacContracts.EvaluationResult{Granted: true}
}

func VerifyCrossTenantFederationHandshake(hs *rbacContracts.FederationHandshake, sourceTenant string, targetTenant string, hmacSecret string, now time.Time) bool {
	if hs == nil || hmacSecret == "" || hs.Signature == "" {
		return false
	}
	if hs.SourceTenantID != sourceTenant || hs.TargetTenantID != targetTenant {
		return false
	}
	if hs.ExpiresAtUnix <= now.Unix() {
		return false
	}
	mac := hmac.New(sha256.New, []byte(hmacSecret))
	mac.Write([]byte(fmt.Sprintf("%s|%s|%d", hs.SourceTenantID, hs.TargetTenantID, hs.ExpiresAtUnix)))
	expected := mac.Sum(nil)
	provided, err := hex.DecodeString(hs.Signature)
	if err != nil {
		return false
	}
	return hmac.Equal(expected, provided)
}

func FlattenRoleHierarchyPermissions(
	roles []rbacContracts.RoleName,
	groupMemberships []string,
	roleDefs map[rbacContracts.RoleName][]rbacContracts.Permission,
	hierarchy map[string][]string,
) map[rbacContracts.RoleName][]rbacContracts.Permission {
	if len(roleDefs) == 0 {
		roleDefs = DefaultRoleDefinitions
	}
	flattened := make(map[rbacContracts.RoleName][]rbacContracts.Permission, len(roleDefs))
	for k, v := range roleDefs {
		copied := make([]rbacContracts.Permission, len(v))
		copy(copied, v)
		flattened[k] = copied
	}
	if len(hierarchy) == 0 {
		return flattened
	}
	seeds := make([]rbacContracts.RoleName, 0, len(roles)+len(groupMemberships))
	seeds = append(seeds, roles...)
	for _, g := range groupMemberships {
		seeds = append(seeds, rbacContracts.RoleName(g))
	}
	for _, root := range seeds {
		visited := map[string]bool{string(root): true}
		queue := []string{string(root)}
		depth := 0
		var merged []rbacContracts.Permission
		for len(queue) > 0 && depth <= MaxRoleHierarchyDepth {
			nextQueue := []string{}
			for _, curr := range queue {
				merged = append(merged, roleDefs[rbacContracts.RoleName(curr)]...)
				for _, child := range hierarchy[curr] {
					if !visited[child] {
						visited[child] = true
						nextQueue = append(nextQueue, child)
					}
				}
			}
			queue = nextQueue
			depth++
		}
		if len(merged) > 0 {
			flattened[root] = merged
		}
	}
	return flattened
}

func BuildPermissionHashset(perms []rbacContracts.Permission) map[string]struct{} {
	set := make(map[string]struct{}, len(perms))
	for _, p := range perms {
		key := strings.ToLower(string(p.Resource)) + ":" + strings.ToLower(string(p.Action))
		set[key] = struct{}{}
	}
	return set
}

func hasPermissionInHashset(set map[string]struct{}, res rbacContracts.Resource, act rbacContracts.Action, allowDestructiveWildcard bool) bool {
	r := strings.ToLower(string(res))
	a := strings.ToLower(string(act))
	if _, ok := set[r+":"+a]; ok {
		return true
	}
	if isDestructiveAction(act) && !allowDestructiveWildcard {
		return false
	}
	if _, ok := set[r+":*"]; ok {
		return true
	}
	if _, ok := set["*:"+a]; ok {
		return true
	}
	_, ok := set["*:*"]
	return ok
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

func matchesPermissionExact(perm rbacContracts.Permission, targetResource rbacContracts.Resource, targetAction rbacContracts.Action) bool {
	return matchesPermission(perm, targetResource, targetAction, true)
}

func isDestructiveAction(action rbacContracts.Action) bool {
	a := strings.ToLower(string(action))
	return a == "delete" || a == "drop" || a == "terminate"
}

func matchesPermission(perm rbacContracts.Permission, targetResource rbacContracts.Resource, targetAction rbacContracts.Action, allowDestructiveWildcard bool) bool {
	resourceMatch := perm.Resource == rbacContracts.ResourceAll ||
		perm.Resource == targetResource ||
		strings.EqualFold(string(perm.Resource), string(targetResource))
	if !resourceMatch {
		return false
	}
	if perm.Action == rbacContracts.ActionAll && isDestructiveAction(targetAction) && !allowDestructiveWildcard {
		return false
	}
	actionMatch := perm.Action == rbacContracts.ActionAll ||
		perm.Action == targetAction ||
		strings.EqualFold(string(perm.Action), string(targetAction))
	return actionMatch
}

func EvaluateTaskExecutionLease(req rbacContracts.TaskExecutionLeaseRequest) rbacContracts.TaskExecutionLeaseResult {
	if !req.TokenExpired {
		return rbacContracts.TaskExecutionLeaseResult{
			Granted:   true,
			Directive: "NORMAL",
			Reason:    "token valid; proceed with standard verification",
		}
	}
	if !req.IsIdempotentBatchStep {
		return rbacContracts.TaskExecutionLeaseResult{
			Granted:       false,
			Directive:     "ABORT",
			Reason:        "token expired during non-idempotent operation",
			ViolationCode: "ERR_TOKEN_EXPIRED",
		}
	}
	if req.HasSignedTaskLease && !req.LeaseExpired {
		return rbacContracts.TaskExecutionLeaseResult{
			Granted:   true,
			Directive: "ALLOW_LEASE",
			Reason:    "cryptographically signed task execution lease is valid",
		}
	}
	if req.RefreshSucceeded {
		return rbacContracts.TaskExecutionLeaseResult{
			Granted:   true,
			Directive: "RESUME_RENEWED_LEASE",
			Reason:    "token refreshed via platform agent; resume with renewed lease",
		}
	}
	return rbacContracts.TaskExecutionLeaseResult{
		Granted:       false,
		Directive:     "ABORT",
		Reason:        "task lease missing or expired and refresh failed; graceful compensation required",
		ViolationCode: "ERR_TASK_LEASE_EXPIRED",
	}
}

func EvaluateAsyncJobRevocationDrift(
	ctx context.Context,
	subjectRevoked bool,
	queueLatency time.Duration,
	oidcIntrospect func(ctx context.Context) (bool, error),
) rbacContracts.AsyncJobRevocationResult {
	if subjectRevoked {
		return rbacContracts.AsyncJobRevocationResult{
			Granted:       false,
			Directive:     "DISCARD",
			Reason:        "subject present in revocation register; poison job terminated",
			ViolationCode: "ERR_SUBJECT_REVOKED",
		}
	}
	if queueLatency > MaxPermissibleQueueDrift {
		if oidcIntrospect == nil {
			return rbacContracts.AsyncJobRevocationResult{
				Granted:       false,
				Directive:     "DISCARD",
				Reason:        "queue drift exceeded 5m and OIDC introspect callback is unavailable",
				ViolationCode: "ERR_OIDC_INTROSPECT_UNAVAILABLE",
			}
		}
		active, err := oidcIntrospect(ctx)
		if err != nil || !active {
			return rbacContracts.AsyncJobRevocationResult{
				Granted:       false,
				Directive:     "DISCARD",
				Reason:        "OIDC introspection reported inactive or unreachable subject after queue drift",
				ViolationCode: "ERR_OIDC_INACTIVE",
			}
		}
	}
	return rbacContracts.AsyncJobRevocationResult{
		Granted:   true,
		Directive: "EXECUTE",
		Reason:    "async job dispatched",
	}
}

func EvaluateRegionalSplitBrain(globalAuthorityReachable bool, isRoleMutation bool, snapshotAge time.Duration) rbacContracts.RegionalSplitBrainResult {
	if globalAuthorityReachable {
		return rbacContracts.RegionalSplitBrainResult{
			Granted:    true,
			HTTPStatus: http.StatusOK,
			Reason:     "global policy authority reachable",
		}
	}
	if isRoleMutation {
		return rbacContracts.RegionalSplitBrainResult{
			Granted:       false,
			HTTPStatus:    http.StatusServiceUnavailable,
			Reason:        "cannot mutate roles during regional network partition",
			ViolationCode: "ERR_SPLIT_BRAIN_MUTATION_BLOCKED",
		}
	}
	if snapshotAge >= 0 && snapshotAge <= SplitBrainStaleGraceWindow {
		return rbacContracts.RegionalSplitBrainResult{
			Granted:       true,
			Degraded:      true,
			HTTPStatus:    http.StatusOK,
			WarningHeader: "299 - Degraded Regional Policy Snapshot",
			Reason:        "evaluated against local read-replica snapshot within 2h grace window",
		}
	}
	return rbacContracts.RegionalSplitBrainResult{
		Granted:       false,
		HTTPStatus:    http.StatusForbidden,
		Reason:        "regional policy snapshot expired beyond 2h stale grace window",
		ViolationCode: "ERR_SNAPSHOT_EXPIRED",
	}
}

func EvaluateExclusiveRoleClaim(rowsAffected int64) error {
	if rowsAffected != 1 {
		return errors.NewConflictError("role binding already claimed")
	}
	return nil
}

func EvaluateStreamingSessionHeartbeat(grantExpiresAt time.Time, now time.Time, sessionStartedAt time.Time) rbacContracts.StreamingHeartbeatResult {
	duration := now.Sub(sessionStartedAt)
	if duration < 0 {
		duration = 0
	}
	if grantExpiresAt.IsZero() || !now.Before(grantExpiresAt) {
		return rbacContracts.StreamingHeartbeatResult{
			Terminate:       true,
			SessionDuration: duration,
			Reason:          "JIT grant or session window expired; terminate socket and flush TTY buffer",
		}
	}
	return rbacContracts.StreamingHeartbeatResult{
		Terminate:       false,
		SessionDuration: duration,
		Reason:          "session active",
	}
}

func EvaluateSidecarPDPFallback(sidecarResult *rbacContracts.SidecarDecisionResult, req rbacContracts.EvaluationRequest, now time.Time) rbacContracts.EvaluationResult {
	if sidecarResult != nil {
		if sidecarResult.Allowed {
			return rbacContracts.EvaluationResult{
				Granted: true,
				Reason:  "permitted by local sidecar PDP (" + string(sidecarResult.Engine) + ")",
			}
		}
		return rbacContracts.EvaluationResult{
			Granted:       false,
			Reason:        sidecarResult.Reason,
			ViolationCode: sidecarResult.ViolationCode,
		}
	}
	return EvaluateAuthorization(req, now)
}

func EvaluateEBPFHookEnforcement(event rbacContracts.EBPFHookEvent, policy rbacContracts.EBPFHookPolicy) error {
	if !policy.Enforce {
		return nil
	}
	for _, denied := range policy.DeniedPaths {
		if denied != "" && strings.HasPrefix(event.TargetPath, denied) {
			return errors.NewForbiddenError("ebpf hook blocked access to protected path: " + event.TargetPath)
		}
	}
	if len(policy.AllowedSyscalls) > 0 {
		allowed := false
		for _, sc := range policy.AllowedSyscalls {
			if sc == event.Syscall {
				allowed = true
				break
			}
		}
		if !allowed {
			return errors.NewForbiddenError("ebpf hook blocked disallowed syscall: " + event.Syscall)
		}
	}
	return nil
}

func AnalyzeLeastPrivilege(roles map[rbacContracts.RoleName][]rbacContracts.Permission, usage []rbacContracts.PermissionUsageRecord, staleWindow time.Duration, now time.Time) rbacContracts.LeastPrivilegeReport {
	lastUsedMap := make(map[string]time.Time, len(usage))
	for _, u := range usage {
		key := string(u.Role) + ":" + string(u.Resource) + ":" + string(u.Action)
		if existing, ok := lastUsedMap[key]; !ok || u.LastUsedAt.After(existing) {
			lastUsedMap[key] = u.LastUsedAt
		}
	}
	findings := make([]rbacContracts.UnusedPermissionFinding, 0)
	for role, perms := range roles {
		for _, perm := range perms {
			key := string(role) + ":" + string(perm.Resource) + ":" + string(perm.Action)
			lastUsed, seen := lastUsedMap[key]
			if !seen {
				findings = append(findings, rbacContracts.UnusedPermissionFinding{
					Role:       role,
					Permission: perm,
					Reason:     "permission has zero recorded usage",
				})
				continue
			}
			if staleWindow > 0 && now.Sub(lastUsed) > staleWindow {
				findings = append(findings, rbacContracts.UnusedPermissionFinding{
					Role:       role,
					Permission: perm,
					LastUsedAt: lastUsed,
					Reason:     "permission unused beyond stale window",
				})
			}
		}
	}
	return rbacContracts.LeastPrivilegeReport{
		EvaluatedAt: now,
		TotalRoles:  len(roles),
		Findings:    findings,
	}
}

func PruneUnusedPermissions(perms []rbacContracts.Permission, findings []rbacContracts.UnusedPermissionFinding) []rbacContracts.Permission {
	pruneSet := make(map[string]struct{}, len(findings))
	for _, f := range findings {
		pruneSet[string(f.Permission.Resource)+":"+string(f.Permission.Action)] = struct{}{}
	}
	kept := make([]rbacContracts.Permission, 0, len(perms))
	for _, p := range perms {
		if _, shouldPrune := pruneSet[string(p.Resource)+":"+string(p.Action)]; !shouldPrune {
			kept = append(kept, p)
		}
	}
	return kept
}
