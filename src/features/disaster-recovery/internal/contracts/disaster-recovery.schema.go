// Package contracts provides schema validation and Anti-Corruption Layer (ACL) logic for Disaster Recovery.
//
// Architectural Blueprint:
// - Step 03 Schema: Validates incoming commands, bounds inputs against gold-standard floors, and normalizes strings.
// - Ensures clean boundaries between API representation and pure domain types.
// - Enforces:
//   - B-1 Residency: distinct regions.
//   - B-2 RTO/RPO floors per tier:
//   - TIER_0: RTO floor 300s (5m), RPO floor 0s.
//   - TIER_1: RTO floor 1800s (30m), RPO floor 300s (5m).
//   - TIER_2: RTO floor 14400s (4h), RPO floor 3600s (1h).
//
// - 100% comment-free function bodies.
package contracts

import (
	"regexp"
	"strings"

	"internal-developer-platform/src/shared/errors"
)

var (
	rxGenericID  = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]{1,128}$`)
	rxRegionName = regexp.MustCompile(`^[a-z]+-[a-z0-9]+[0-9]?$`)
)

func ValidateTenantID(tenantID string) error {
	if tenantID == "" || !rxGenericID.MatchString(tenantID) {
		return errors.NewValidationError("tenantId header is required and must match identifier format")
	}
	return nil
}

func ValidateCreateDRPlanCommand(cmd CreateDRPlanCommand) error {
	if cmd.TenantID == "" || !rxGenericID.MatchString(cmd.TenantID) {
		return errors.NewValidationError("tenantId must match alphanumeric identifier format")
	}
	if cmd.PlanName == "" || len(strings.TrimSpace(cmd.PlanName)) == 0 {
		return errors.NewValidationError("planName must be non-empty")
	}
	if cmd.PrimaryRegion == "" || !rxRegionName.MatchString(cmd.PrimaryRegion) {
		return errors.NewValidationError("primaryRegion must be valid region identifier")
	}
	if cmd.DRRegion == "" || !rxRegionName.MatchString(cmd.DRRegion) {
		return errors.NewValidationError("drRegion must be valid region identifier")
	}
	if cmd.PrimaryRegion == cmd.DRRegion {
		return errors.NewValidationError("primaryRegion and drRegion must be distinct under B-1 residency rules")
	}
	if cmd.PrimaryClusterID == "" || cmd.DRClusterID == "" {
		return errors.NewValidationError("primaryClusterId and drClusterId must both be provided")
	}
	if cmd.PrimaryClusterID == cmd.DRClusterID {
		return errors.NewValidationError("drClusterId must be distinct from primaryClusterId")
	}
	if cmd.IdempotencyKey == "" || len(cmd.IdempotencyKey) > 256 {
		return errors.NewValidationError("idempotencyKey must be between 1 and 256 characters")
	}

	return validateFloorsForTier(cmd.IncidentTier, cmd.TargetRTOSeconds, cmd.TargetRPOSeconds)
}

func validateFloorsForTier(tier string, rto int, rpo int) error {
	switch tier {
	case "TIER_0":
		if rto < 0 || rto > 300 {
			return errors.NewValidationError("tier 0 RTO cannot exceed 300 seconds (5m floor)")
		}
		if rpo != 0 {
			return errors.NewValidationError("tier 0 RPO must be 0 seconds (zero data loss mandate)")
		}
	case "TIER_1":
		if rto < 0 || rto > 1800 {
			return errors.NewValidationError("tier 1 RTO cannot exceed 1800 seconds (30m floor)")
		}
		if rpo < 0 || rpo > 300 {
			return errors.NewValidationError("tier 1 RPO cannot exceed 300 seconds (5m floor)")
		}
	case "TIER_2":
		if rto < 0 || rto > 14400 {
			return errors.NewValidationError("tier 2 RTO cannot exceed 14400 seconds (4h floor)")
		}
		if rpo < 0 || rpo > 3600 {
			return errors.NewValidationError("tier 2 RPO cannot exceed 3600 seconds (1h floor)")
		}
	default:
		return errors.NewValidationError("incidentTier must be TIER_0, TIER_1, or TIER_2")
	}
	return nil
}

func ValidateDeclareIncidentCommand(cmd DeclareIncidentCommand) error {
	if cmd.TenantID == "" || !rxGenericID.MatchString(cmd.TenantID) {
		return errors.NewValidationError("tenantId is required")
	}
	if cmd.PlanID == "" || !rxGenericID.MatchString(cmd.PlanID) {
		return errors.NewValidationError("planId is required")
	}
	if cmd.Declarer == "" || len(strings.TrimSpace(cmd.Declarer)) == 0 {
		return errors.NewValidationError("declarer identity is required")
	}
	if len(cmd.EvidenceLinks) == 0 {
		return errors.NewValidationError("at least one evidence link is mandatory for declaration")
	}
	return nil
}

func ValidateInitiateFailoverCommand(cmd InitiateFailoverCommand) error {
	if cmd.TenantID == "" || !rxGenericID.MatchString(cmd.TenantID) {
		return errors.NewValidationError("tenantId is required")
	}
	if cmd.PlanID == "" || !rxGenericID.MatchString(cmd.PlanID) {
		return errors.NewValidationError("planId is required")
	}
	if cmd.IncidentID == "" || !rxGenericID.MatchString(cmd.IncidentID) {
		return errors.NewValidationError("incidentId is required")
	}
	if cmd.Initiator == "" || len(strings.TrimSpace(cmd.Initiator)) == 0 {
		return errors.NewValidationError("initiator identity is required")
	}
	if cmd.Mode != FailoverModeSwitchover && cmd.Mode != FailoverModeFailover {
		return errors.NewValidationError("mode must be SWITCHOVER or FAILOVER")
	}
	return nil
}

func ValidateExecuteFailbackCommand(cmd ExecuteFailbackCommand) error {
	if cmd.TenantID == "" || !rxGenericID.MatchString(cmd.TenantID) {
		return errors.NewValidationError("tenantId is required")
	}
	if cmd.PlanID == "" || !rxGenericID.MatchString(cmd.PlanID) {
		return errors.NewValidationError("planId is required")
	}
	if cmd.Initiator == "" || cmd.Approver == "" {
		return errors.NewValidationError("initiator and approver are mandatory for failback (two-person rule)")
	}
	if cmd.Initiator == cmd.Approver {
		return errors.NewConflictError("failback initiator and approver must be distinct individuals")
	}
	if cmd.Divergence > 0 {
		return errors.NewConflictError("failback rejected: divergence must be zero (reconciliation report)")
	}
	if cmd.ReverseLag > 0 {
		return errors.NewConflictError("failback rejected: reverse replication lag must be zero")
	}
	return nil
}

func ValidateFenceDatasetCommand(cmd FenceDatasetCommand) error {
	if cmd.DatasetID == "" || len(strings.TrimSpace(cmd.DatasetID)) == 0 {
		return errors.NewValidationError("datasetId is required")
	}
	if cmd.Authority == "" {
		return errors.NewValidationError("authority is required")
	}
	return nil
}

func ValidateInitiateRestoreCommand(cmd InitiateRestoreCommand) error {
	if cmd.TenantID == "" || !rxGenericID.MatchString(cmd.TenantID) {
		return errors.NewValidationError("tenantId is required")
	}
	if cmd.BackupID == "" || len(strings.TrimSpace(cmd.BackupID)) == 0 {
		return errors.NewValidationError("backupId is required")
	}
	if cmd.TargetCluster == "" || len(strings.TrimSpace(cmd.TargetCluster)) == 0 {
		return errors.NewValidationError("targetCluster is required")
	}
	if cmd.Namespace == "" || len(strings.TrimSpace(cmd.Namespace)) == 0 {
		return errors.NewValidationError("namespace is required")
	}
	return nil
}

func ValidateActivateBreakglassCommand(cmd ActivateBreakglassCommand) error {
	if cmd.TenantID == "" || !rxGenericID.MatchString(cmd.TenantID) {
		return errors.NewValidationError("tenantId is required")
	}
	if cmd.Requester == "" || cmd.Approver == "" {
		return errors.NewValidationError("requester and approver are required (two-person rule)")
	}
	if cmd.Requester == cmd.Approver {
		return errors.NewConflictError("requester and approver must be distinct individuals")
	}
	if cmd.DurationMins <= 0 || cmd.DurationMins > 240 {
		return errors.NewValidationError("durationMins must be between 1 and 240 minutes")
	}
	if cmd.Justification == "" || len(strings.TrimSpace(cmd.Justification)) < 10 {
		return errors.NewValidationError("justification must be at least 10 characters")
	}
	return nil
}
