/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING RULES (V.3 role 07)

 1. Role:
    - Pure stateless decision rules for INV-04 atomic reservations, INV-05 IP space quota, INV-15 tenant isolation, and EC-03 name normalization.
 2. Invariants:
    - Zero IO, zero mutable state, zero inline comments below this header.
*/
package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/shared/errors"
)

func NormalizeResourceName(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func DefaultTenantCapacityCeiling(tenantID string) contracts.TenantCapacityCeiling {
	return contracts.TenantCapacityCeiling{
		TenantID:            tenantID,
		MaxQuotaUnits:       100,
		MaxIPCIDRBlocks:     64,
		MaxClusterIPUtilPct: 80,
	}
}

func ValidateCapacityReservation(
	quotaUsed int,
	ipUsed int,
	req contracts.ReservationRequest,
	ceiling contracts.TenantCapacityCeiling,
) error {
	if NormalizeResourceName(req.RawName) == "" && NormalizeResourceName(req.NormalizedName) == "" {
		return errors.NewValidationError("resource name is required for reservation")
	}
	if req.QuotaUnits <= 0 {
		return errors.NewValidationError("quotaUnits must be positive")
	}
	if req.IPCIDRBlocks < 0 {
		return errors.NewValidationError("ipCidrBlocks must be non-negative")
	}
	if ceiling.MaxClusterIPUtilPct > 0 && req.ClusterIPUtilPct > ceiling.MaxClusterIPUtilPct {
		return errors.NewConflictError(fmt.Sprintf(
			"cluster IP utilization %d%% exceeds cfg.ip.max_util ceiling %d%%",
			req.ClusterIPUtilPct,
			ceiling.MaxClusterIPUtilPct,
		))
	}
	if quotaUsed+req.QuotaUnits > ceiling.MaxQuotaUnits {
		return errors.NewConflictError(fmt.Sprintf(
			"tenant %s quota ceiling exceeded (%d + %d > %d)",
			req.TenantID,
			quotaUsed,
			req.QuotaUnits,
			ceiling.MaxQuotaUnits,
		))
	}
	if ipUsed+req.IPCIDRBlocks > ceiling.MaxIPCIDRBlocks {
		return errors.NewConflictError(fmt.Sprintf(
			"tenant %s IP CIDR block ceiling exceeded (%d + %d > %d)",
			req.TenantID,
			ipUsed,
			req.IPCIDRBlocks,
			ceiling.MaxIPCIDRBlocks,
		))
	}
	return nil
}

func ComputePlanHash(plan contracts.PinnedPlan) string {
	canonical := strings.Join([]string{
		strings.TrimSpace(plan.TemplateSHA),
		strings.TrimSpace(plan.PolicySHA),
		strings.TrimSpace(plan.Sizing),
		NormalizeResourceName(plan.NormalizedName),
	}, "|")
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

func ValidateApprovalAndPin(
	approval contracts.ApprovalRecord,
	expectedPlan contracts.PinnedPlan,
	periodBudgetUsed int,
	maxPeriodBudget int,
	now time.Time,
) error {
	if strings.TrimSpace(approval.RequesterID) == "" || strings.TrimSpace(approval.ApproverID) == "" {
		return errors.NewForbiddenError("requester and approver identities are required")
	}
	if strings.EqualFold(strings.TrimSpace(approval.RequesterID), strings.TrimSpace(approval.ApproverID)) {
		return errors.NewForbiddenError("self-approval is forbidden: approver must differ from requester")
	}
	if approval.Status != "APPROVED" {
		return errors.NewConflictError(fmt.Sprintf("approval %s is not in APPROVED state (%s)", approval.ID, approval.Status))
	}
	if !approval.ExpiresAt.After(now) {
		return errors.NewConflictError(fmt.Sprintf("approval %s has expired", approval.ID))
	}
	expectedHash := ComputePlanHash(expectedPlan)
	if approval.PlanHash != expectedHash {
		return errors.NewConflictError("execution plan hash mismatch: template, policy, sizing, or name changed after approval")
	}
	if maxPeriodBudget > 0 && periodBudgetUsed+approval.BudgetUnits > maxPeriodBudget {
		return errors.NewConflictError(fmt.Sprintf(
			"tenant %s aggregate approval budget exceeded (%d + %d > %d)",
			approval.TenantID,
			periodBudgetUsed,
			approval.BudgetUnits,
			maxPeriodBudget,
		))
	}
	return nil
}

func ValidateDataSafeCompensation(isDataBearing bool, hasRetentionDecision bool, isEphemeral bool) error {
	if isDataBearing && isEphemeral {
		return errors.NewConflictError("EC-14: ephemeral environment TTL cannot delete attached data-bearing resource")
	}
	if isDataBearing && !hasRetentionDecision {
		return errors.NewConflictError("INV-06: data-bearing resource requires recorded retention decision before compensation or deletion")
	}
	return nil
}

func BuildTeardownDAGOrder() []string {
	return []string{"DNS", "LB_ENDPOINT", "RESOURCE"}
}

func ValidateSingleMutationPath(principalID string, viaOwningAPI bool) error {
	if !viaOwningAPI {
		return errors.NewForbiddenError(fmt.Sprintf("INV-08: direct cloud/cluster mutation by %s outside owning API -> Git -> Argo CD is forbidden", principalID))
	}
	return nil
}

func ClassifyStepError(statusCode int, retryAfterSeconds int, attempt int, maxAttempts int) (bool, int) {
	if attempt >= maxAttempts {
		return false, 0
	}
	switch statusCode {
	case 400, 401, 403, 404, 422:
		return false, 0
	case 429:
		if retryAfterSeconds > 0 {
			return true, retryAfterSeconds
		}
		return true, 60
	default:
		if statusCode >= 500 || statusCode == 0 {
			backoff := 1 << uint(attempt-1)
			if backoff > 60 {
				backoff = 60
			}
			return true, backoff
		}
		return false, 0
	}
}

func EvaluateObservedReady(syncStatus string, healthStatus string, availableReplicas int) (bool, string) {
	if syncStatus != "Synced" {
		return false, fmt.Sprintf("argocd sync not Synced (%s)", syncStatus)
	}
	if healthStatus != "Healthy" {
		return false, fmt.Sprintf("argocd health not Healthy (%s)", healthStatus)
	}
	if availableReplicas < 1 {
		return false, "zero available replicas observed"
	}
	return true, ""
}

func ScanManifestsForSecrets(files map[string]string) error {
	markers := []string{
		"-----BEGIN RSA PRIVATE KEY-----",
		"-----BEGIN PRIVATE KEY-----",
		"ghp_",
		"AKIA",
		"AIza",
	}
	for path, content := range files {
		for _, marker := range markers {
			if strings.Contains(content, marker) {
				return errors.NewForbiddenError(fmt.Sprintf("INV-13: secret material detected in rendered manifest %s", path))
			}
		}
	}
	return nil
}

func ValidateTraceabilityLabels(requestID string, owner string, costCenter string, userID string) error {
	if strings.TrimSpace(requestID) == "" || strings.TrimSpace(owner) == "" || strings.TrimSpace(costCenter) == "" || strings.TrimSpace(userID) == "" {
		return errors.NewValidationError("INV-14: requestId, owner, costCenter, and userId are mandatory for traceability")
	}
	return nil
}

func SelectAlternateZoneOrFamily(failedZone string, allowedZones []string) (string, error) {
	for _, z := range allowedZones {
		if z != "" && z != failedZone {
			return z, nil
		}
	}
	return "", errors.NewConflictError("EC-06: no alternate zone available after stockout")
}
