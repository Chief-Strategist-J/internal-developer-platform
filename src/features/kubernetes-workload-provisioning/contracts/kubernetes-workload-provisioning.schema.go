package contracts

import (
	"fmt"
	"regexp"
	"strings"

	"internal-developer-platform/src/shared/errors"
)

var (
	rxWorkloadID        = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]{1,128}$`)
	rxK8sName           = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?$`)
	rxCPUQuantity       = regexp.MustCompile(`^[0-9]+m?$`)
	rxMemoryQuantity    = regexp.MustCompile(`^[0-9]+(Mi|Gi|Ki|M|G|K)?$`)
	rxImageRef          = regexp.MustCompile(`^[a-zA-Z0-9_\.\-\/]+(:[a-zA-Z0-9_\.\-]+)?(@sha256:[a-f0-9]{64})?$`)
)

func ValidateWorkloadCommand(cmd WorkloadProvisionCommand) error {
	if cmd.ID == "" || !rxWorkloadID.MatchString(cmd.ID) {
		return errors.NewValidationError("id must match alphanumeric identifier format")
	}
	if cmd.TenantID == "" || !rxWorkloadID.MatchString(cmd.TenantID) {
		return errors.NewValidationError("tenantId must match alphanumeric identifier format")
	}
	if cmd.ServiceID == "" || !rxWorkloadID.MatchString(cmd.ServiceID) {
		return errors.NewValidationError("serviceId must match alphanumeric identifier format")
	}
	if cmd.Namespace == "" || !rxK8sName.MatchString(cmd.Namespace) {
		return errors.NewValidationError("namespace must be RFC-1123 compliant")
	}
	if cmd.WorkloadName == "" || !rxK8sName.MatchString(cmd.WorkloadName) {
		return errors.NewValidationError("workloadName must be RFC-1123 compliant")
	}
	if cmd.Image == "" || !rxImageRef.MatchString(cmd.Image) {
		return errors.NewValidationError("image must be a valid container image reference")
	}
	if cmd.CPURequest != "" && !rxCPUQuantity.MatchString(cmd.CPURequest) {
		return errors.NewValidationError("cpuRequest format is invalid")
	}
	if cmd.CPULimit != "" && !rxCPUQuantity.MatchString(cmd.CPULimit) {
		return errors.NewValidationError("cpuLimit format is invalid")
	}
	if cmd.MemoryRequest != "" && !rxMemoryQuantity.MatchString(cmd.MemoryRequest) {
		return errors.NewValidationError("memoryRequest format is invalid")
	}
	if cmd.MemoryLimit != "" && !rxMemoryQuantity.MatchString(cmd.MemoryLimit) {
		return errors.NewValidationError("memoryLimit format is invalid")
	}
	if cmd.Replicas < 1 || cmd.Replicas > 100 {
		return errors.NewValidationError("replicas must be between 1 and 100")
	}
	if cmd.IdempotencyKey == "" || len(cmd.IdempotencyKey) > 256 {
		return errors.NewValidationError("idempotencyKey must be between 1 and 256 characters")
	}
	if cmd.ImagePullPolicy != "" && cmd.ImagePullPolicy != "IfNotPresent" && cmd.ImagePullPolicy != "Always" && cmd.ImagePullPolicy != "Never" {
		return errors.NewValidationError("imagePullPolicy must be IfNotPresent, Always, or Never")
	}
	if len(cmd.BreakglassJustification) > 1024 {
		return errors.NewValidationError("breakglassJustification must not exceed 1024 characters")
	}
	if err := ValidateOnCallAndRunbook(cmd.IncidentTier, cmd.OnCallPager, cmd.RunbookURL); err != nil {
		return err
	}
	return nil
}

func ValidateOnCallAndRunbook(tier IncidentTier, pager, runbookURL string) error {
	normalizedTier := tier
	if normalizedTier == "" {
		normalizedTier = IncidentTier2
	}
	switch normalizedTier {
	case IncidentTier0, IncidentTier1, IncidentTier2, IncidentTier3:
	default:
		return errors.NewValidationError(fmt.Sprintf("invalid incident tier: %s", tier))
	}
	if pager != "" && runbookURL == "" {
		return errors.NewValidationError("V-27 violation: 'no runbook no page' rule breached - on-call pager requires a valid runbookUrl")
	}
	if (normalizedTier == IncidentTier0 || normalizedTier == IncidentTier1) && pager == "" {
		return errors.NewValidationError("V-27 violation: tier-0 and tier-1 workloads require 24/7 on-call paging target")
	}
	if runbookURL != "" && !strings.HasPrefix(runbookURL, "http://") && !strings.HasPrefix(runbookURL, "https://") {
		return errors.NewValidationError("V-27 violation: runbookUrl must be a valid http or https url")
	}
	return nil
}
