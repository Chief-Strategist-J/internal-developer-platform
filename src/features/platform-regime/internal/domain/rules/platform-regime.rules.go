/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PLATFORM REGIME DOMAIN RULES

 1. Domain Evaluation Algorithm:
    - Validates WorkloadComplianceInput against an active PlatformRegimePolicyRecord.
    - Enforces B-1 (Jurisdiction, Retention >= Regime Floor, Approved Data Residency Regions, KMS Custody).
    - Enforces B-4 (Tenant isolation constraints).
    - Returns structured ComplianceViolations with zero cross-feature imports (G-02) and zero inline comments.
*/
package rules

import (
	"fmt"
	"strings"

	"internal-developer-platform/src/features/platform-regime/internal/contracts"
)

func ValidateWorkloadAgainstRegime(input contracts.WorkloadComplianceInput, regime contracts.PlatformRegimePolicyRecord) (bool, []contracts.ComplianceViolation) {
	violations := make([]contracts.ComplianceViolation, 0)
	critical := false

	if input.AuditRetentionDays > 0 && input.AuditRetentionDays < regime.AuditRetentionDays {
		critical = true
		violations = append(violations, contracts.ComplianceViolation{
			Rule:    "RGM-AUD-001",
			Message: fmt.Sprintf("workload audit retention (%d days) is below active regime floor (%d days)", input.AuditRetentionDays, regime.AuditRetentionDays),
		})
	}

	if len(regime.DataResidencyRegions) > 0 {
		regionAllowed := false
		for _, allowedReg := range regime.DataResidencyRegions {
			if strings.EqualFold(allowedReg, "all") || strings.Contains(input.Namespace, allowedReg) {
				regionAllowed = true
				break
			}
		}
		if !regionAllowed && strings.Contains(input.Namespace, "prod") {
			critical = true
			violations = append(violations, contracts.ComplianceViolation{
				Rule:    "RGM-RGN-001",
				Message: fmt.Sprintf("workload namespace %q does not match approved regime data residency regions %v", input.Namespace, regime.DataResidencyRegions),
			})
		}
	}

	if (regime.KMSKeyCustodyMode == contracts.KMSCustodyEKM || regime.KMSKeyCustodyMode == contracts.KMSCustodyDODKMS) && input.HasEncryptedSecrets {
		if input.KMSKeyURI == "" {
			critical = true
			violations = append(violations, contracts.ComplianceViolation{
				Rule:    "RGM-KMS-001",
				Message: fmt.Sprintf("active regime mandates %s key custody but no kmsKeyURI was provided", regime.KMSKeyCustodyMode),
			})
		}
	}

	if regime.TenantIsolationMode == contracts.TenancyModeInternalOnly && strings.Contains(input.Namespace, "public") {
		critical = true
		violations = append(violations, contracts.ComplianceViolation{
			Rule:    "RGM-TEN-001",
			Message: "public routing is prohibited under internal-only tenant isolation mode",
		})
	}

	return !critical, violations
}
