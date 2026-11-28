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
