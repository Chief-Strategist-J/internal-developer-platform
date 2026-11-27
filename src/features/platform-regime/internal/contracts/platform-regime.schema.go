/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PLATFORM REGIME SCHEMA VALIDATOR

1. Closed Schema Ingress Gate:
  - Validates all fields against domain constraints before hitting services.
  - Enforces RFC-1123 formats, minimum floor retention >= 30, and positive RTO budgets.
  - Zero inline comments inside function bodies.
*/
package contracts

import (
	"regexp"
	"strings"

	"internal-developer-platform/src/shared/errors"
)

var (
	rxAlphanumericID = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]{1,128}$`)
	rxRegionName     = regexp.MustCompile(`^[a-z0-9\-]{2,32}$`)
)

func ValidateCreateRegimePolicyCommand(cmd CreateRegimePolicyCommand) error {
	if strings.TrimSpace(cmd.TenantID) == "" || !rxAlphanumericID.MatchString(cmd.TenantID) {
		return errors.NewValidationError("tenantId must match alphanumeric identifier format")
	}
	if strings.TrimSpace(cmd.ExternalID) == "" || !rxAlphanumericID.MatchString(cmd.ExternalID) {
		return errors.NewValidationError("externalId must match alphanumeric identifier format")
	}
	if strings.TrimSpace(cmd.Jurisdiction) == "" {
		return errors.NewValidationError("jurisdiction must not be empty")
	}
	if cmd.AuditRetentionDays < 30 {
		return errors.NewValidationError("auditRetentionDays cannot be less than the platform floor of 30 days")
	}
	if len(cmd.DataResidencyRegions) == 0 {
		return errors.NewValidationError("dataResidencyRegions must contain at least one approved region")
	}
	for _, reg := range cmd.DataResidencyRegions {
		if !rxRegionName.MatchString(reg) {
			return errors.NewValidationError("invalid region format in dataResidencyRegions")
		}
	}
	switch cmd.KMSKeyCustodyMode {
	case KMSCustodyCMEK, KMSCustodyEKM, KMSCustodyDODKMS:
	default:
		return errors.NewValidationError("invalid kmsKeyCustodyMode; must be CMEK, EKM, or DOD_KMS")
	}
	if cmd.RTOMinutesTier0 <= 0 || cmd.RTOMinutesTier1 <= 0 {
		return errors.NewValidationError("rtoMinutes must be strictly positive")
	}
	if cmd.RPOMinutesTier0 < 0 || cmd.RPOMinutesTier1 < 0 {
		return errors.NewValidationError("rpoMinutes cannot be negative")
	}
	switch cmd.ClusterMode {
	case ClusterModeGKEStandard, ClusterModeGKEAutopilot:
	default:
		return errors.NewValidationError("invalid clusterMode; must be GKE_STANDARD or GKE_AUTOPILOT")
	}
	if cmd.PrimaryRegionCount < 1 || cmd.StandbyRegionCount < 1 {
		return errors.NewValidationError("primary and standby region count must be at least 1")
	}
	switch cmd.TenantIsolationMode {
	case TenancyModeInternalOnly, TenancyModeSharedExternal:
	default:
		return errors.NewValidationError("invalid tenantIsolationMode; must be INTERNAL_ONLY or SHARED_EXTERNAL")
	}
	return nil
}
