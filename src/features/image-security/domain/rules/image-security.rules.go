package rules

import (
	"fmt"
	"strings"
	"time"

	"internal-developer-platform/src/features/image-security/contracts"
)

func EvaluateImageSecurityPolicy(cmd contracts.ImageScanCommand, now time.Time) contracts.ImageAdmissionVerdict {
	if len(cmd.SecretsFound) > 0 {
		return contracts.ImageAdmissionVerdict{
			Admitted:         false,
			ScanStatus:       contracts.ScanStatusBlocked,
			BlockReason:      fmt.Sprintf("CRITICAL security violation: %d embedded secrets or private keys detected in image", len(cmd.SecretsFound)),
			SecretsCount:     len(cmd.SecretsFound),
			CriticalCVECount: 0,
			HighCVECount:     0,
			HasValidSBOM:     cmd.SBOM != nil,
			WaiverApplied:    false,
		}
	}

	isProd := strings.Contains(strings.ToLower(cmd.Namespace), "prod")
	if isProd && (cmd.SBOM == nil || strings.TrimSpace(cmd.SBOM.Signature) == "") {
		return contracts.ImageAdmissionVerdict{
			Admitted:         false,
			ScanStatus:       contracts.ScanStatusBlocked,
			BlockReason:      "P0-02 violation: production workloads require cryptographically signed SBOM attestation",
			SecretsCount:     0,
			CriticalCVECount: 0,
			HighCVECount:     0,
			HasValidSBOM:     false,
			WaiverApplied:    false,
		}
	}

	criticalWithFix := 0
	criticalZeroDay := 0
	highOldFix := 0
	highCount := 0

	for _, v := range cmd.Vulnerabilities {
		if v.Severity == contracts.SeverityCritical {
			if v.HasFix || strings.TrimSpace(v.FixedVersion) != "" {
				criticalWithFix++
			} else {
				criticalZeroDay++
			}
		} else if v.Severity == contracts.SeverityHigh {
			highCount++
			if v.HasFix && now.Sub(v.PublishedDate) > 30*24*time.Hour {
				highOldFix++
			}
		}
	}

	if criticalWithFix > 0 {
		return contracts.ImageAdmissionVerdict{
			Admitted:         false,
			ScanStatus:       contracts.ScanStatusBlocked,
			BlockReason:      fmt.Sprintf("P0-01 violation: %d CRITICAL CVE(s) with available fixes detected in image", criticalWithFix),
			SecretsCount:     0,
			CriticalCVECount: criticalWithFix + criticalZeroDay,
			HighCVECount:     highCount,
			HasValidSBOM:     cmd.SBOM != nil,
			WaiverApplied:    false,
		}
	}

	if highOldFix > 0 {
		return contracts.ImageAdmissionVerdict{
			Admitted:         false,
			ScanStatus:       contracts.ScanStatusBlocked,
			BlockReason:      fmt.Sprintf("Rule 2 violation: %d HIGH CVE(s) with available fixes older than 30 days exceeded remediation SLA", highOldFix),
			SecretsCount:     0,
			CriticalCVECount: criticalZeroDay,
			HighCVECount:     highCount,
			HasValidSBOM:     cmd.SBOM != nil,
			WaiverApplied:    false,
		}
	}

	if criticalZeroDay > 0 {
		if cmd.ActiveWaiver != nil && cmd.ActiveWaiver.ExpiresAt.After(now) && strings.TrimSpace(cmd.ActiveWaiver.ApprovedBy) != "" {
			return contracts.ImageAdmissionVerdict{
				Admitted:         true,
				ScanStatus:       contracts.ScanStatusException,
				BlockReason:      "",
				CriticalCVECount: criticalZeroDay,
				HighCVECount:     highCount,
				HasValidSBOM:     cmd.SBOM != nil,
				WaiverApplied:    true,
			}
		}

		return contracts.ImageAdmissionVerdict{
			Admitted:         false,
			ScanStatus:       contracts.ScanStatusBlocked,
			BlockReason:      fmt.Sprintf("Tier 3 violation: %d unpatched zero-day CRITICAL CVE(s) without valid active SecOps waiver", criticalZeroDay),
			SecretsCount:     0,
			CriticalCVECount: criticalZeroDay,
			HighCVECount:     highCount,
			HasValidSBOM:     cmd.SBOM != nil,
			WaiverApplied:    false,
		}
	}

	return contracts.ImageAdmissionVerdict{
		Admitted:         true,
		ScanStatus:       contracts.ScanStatusPassed,
		BlockReason:      "",
		SecretsCount:     0,
		CriticalCVECount: 0,
		HighCVECount:     highCount,
		HasValidSBOM:     cmd.SBOM != nil,
		WaiverApplied:    false,
	}
}
