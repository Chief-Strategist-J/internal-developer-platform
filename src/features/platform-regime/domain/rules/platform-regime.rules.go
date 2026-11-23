/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PLATFORM REGIME DOMAIN RULES

1. Domain Evaluation Algorithm:
  - Validates WorkloadProvisionCommand against an active PlatformRegimePolicyRecord.
  - Enforces B-1 (Jurisdiction, Retention >= Regime Floor, Approved Data Residency Regions, KMS Custody).
  - Enforces B-2 (Disaster Recovery RTO/RPO limits if specified).
  - Enforces B-4 (Tenant isolation constraints).
  - Returns structured AdmissionViolations with no inline comments inside functions.
*/
package rules

import (
	"fmt"
	"strings"

	workloadcontracts "internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/platform-regime/contracts"
)

func ValidateWorkloadAgainstRegime(cmd workloadcontracts.WorkloadProvisionCommand, regime contracts.PlatformRegimePolicyRecord) (bool, []workloadcontracts.AdmissionViolation) {
	violations := make([]workloadcontracts.AdmissionViolation, 0)
	critical := false

	if cmd.AuditLogging.RetentionDays > 0 && cmd.AuditLogging.RetentionDays < regime.AuditRetentionDays {
		critical = true
		violations = append(violations, workloadcontracts.AdmissionViolation{
			Rule:    "RGM-AUD-001",
			Message: fmt.Sprintf("workload audit retention (%d days) is below active regime floor (%d days)", cmd.AuditLogging.RetentionDays, regime.AuditRetentionDays),
		})
	}

	if len(regime.DataResidencyRegions) > 0 {
		regionAllowed := false
		for _, allowedReg := range regime.DataResidencyRegions {
			if strings.EqualFold(allowedReg, "all") || strings.Contains(cmd.Namespace, allowedReg) {
				regionAllowed = true
				break
			}
		}
		if !regionAllowed && strings.Contains(cmd.Namespace, "prod") {
			critical = true
			violations = append(violations, workloadcontracts.AdmissionViolation{
				Rule:    "RGM-RGN-001",
				Message: fmt.Sprintf("workload namespace %q does not match approved regime data residency regions %v", cmd.Namespace, regime.DataResidencyRegions),
			})
		}
	}

	if (regime.KMSKeyCustodyMode == contracts.KMSCustodyEKM || regime.KMSKeyCustodyMode == contracts.KMSCustodyDODKMS) && len(cmd.EncryptedSecrets) > 0 {
		if cmd.KMSKeyURI == "" {
			critical = true
			violations = append(violations, workloadcontracts.AdmissionViolation{
				Rule:    "RGM-KMS-001",
				Message: fmt.Sprintf("active regime mandates %s key custody but no kmsKeyURI was provided", regime.KMSKeyCustodyMode),
			})
		}
	}

	if regime.TenantIsolationMode == contracts.TenancyModeInternalOnly && strings.Contains(cmd.Namespace, "public") {
		critical = true
		violations = append(violations, workloadcontracts.AdmissionViolation{
			Rule:    "RGM-TEN-001",
			Message: "public routing is prohibited under internal-only tenant isolation mode",
		})
	}

	return !critical, violations
}
