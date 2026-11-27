package rules

import (
	"fmt"
	"strings"
	"time"

	"internal-developer-platform/src/features/image-security/internal/contracts"
)

func ClassifyVulnerabilityPriority(v contracts.VulnerabilityItem, exposure contracts.WorkloadExposure) contracts.PriorityClass {
	if v.IsBackdoor || strings.Contains(strings.ToUpper(v.CVEID), "BACKDOOR") || strings.Contains(strings.ToUpper(v.CVEID), "MALICIOUS") {
		return contracts.PriorityPrio0
	}
	if v.Exploitation == contracts.ExploitActiveInTheWild {
		return contracts.PriorityPrio0
	}
	if v.Exploitation == contracts.ExploitPublicPOC && (v.HasFix || strings.TrimSpace(v.FixedVersion) != "") {
		return contracts.PriorityPrio1
	}
	if v.Exploitation == contracts.ExploitPublicPOC && exposure == contracts.ExposureInternetFacing {
		return contracts.PriorityPrio1
	}
	if (v.Severity == contracts.SeverityCritical || v.Severity == contracts.SeverityHigh) && (v.HasFix || strings.TrimSpace(v.FixedVersion) != "") {
		return contracts.PriorityPrio2
	}
	if v.Severity == contracts.SeverityMedium || v.Severity == contracts.SeverityLow {
		return contracts.PriorityPrio3
	}
	if v.Severity == "" {
		return contracts.PriorityPrioU
	}
	return contracts.PriorityPrio3
}

func ValidatePodSlotsAdmission(slots []contracts.PodSlotImageRef, isProd bool) (bool, []string) {
	if len(slots) == 0 {
		return true, nil
	}
	violations := make([]string, 0)
	for _, slot := range slots {
		if !strings.Contains(slot.ImageDigest, "sha256:") {
			violations = append(violations, fmt.Sprintf("IMG-02 violation: slot %s (%s) must reference immutable @sha256: digest", slot.Slot, slot.ImageName))
		}
		if isProd && !slot.Attested {
			violations = append(violations, fmt.Sprintf("A01 violation: slot %s (%s) missing verifier attestation", slot.Slot, slot.ImageName))
		}
	}
	if len(violations) > 0 {
		return false, violations
	}
	return true, nil
}

func ValidateImageProvenance(prov *contracts.ProvenanceAttestation, trustedBuilders, repoAllowlist []string) (bool, string) {
	if prov == nil {
		return false, "B01 violation: provenance attestation required"
	}
	if !prov.IsTriggerBuild {
		return false, "B01 violation: manual builds prohibited; trigger-invoked provenance required"
	}
	if len(trustedBuilders) > 0 && !isItemInList(prov.BuilderID, trustedBuilders) {
		return false, fmt.Sprintf("B01 violation: untrusted builder %s", prov.BuilderID)
	}
	if len(repoAllowlist) > 0 && !isItemInList(prov.SourceRepo, repoAllowlist) {
		return false, fmt.Sprintf("B02 violation: source repo %s not in allowlist", prov.SourceRepo)
	}
	if !prov.IsBranchProtected || !prov.IsSignedCommit {
		return false, "B02 violation: commit must be signed and on a protected branch"
	}
	return true, ""
}

func ValidateDigestVEX(vex []contracts.VEXStatement, targetDigest, cveID string, now time.Time) (bool, string) {
	for _, st := range vex {
		if strings.TrimSpace(st.TargetDigest) == "" {
			return false, "D05 violation: image-level VEX prohibited; VEX must be bound to specific digest"
		}
		if st.TargetDigest == targetDigest && st.CVEID == cveID && st.Status == "not_affected" && st.ExpiresAt.After(now) {
			return true, ""
		}
	}
	return false, "no valid active digest-level VEX statement"
}

func isItemInList(item string, list []string) bool {
	for _, candidate := range list {
		if candidate == item {
			return true
		}
	}
	return false
}

func evaluateSlotAndProvenanceGuards(cmd contracts.ImageScanCommand, isProd bool) (bool, string, []string) {
	if len(cmd.PodSlots) > 0 {
		slotOk, slotViolations := ValidatePodSlotsAdmission(cmd.PodSlots, isProd)
		if !slotOk {
			return false, strings.Join(slotViolations, "; "), slotViolations
		}
	}
	if isProd && cmd.Provenance != nil {
		trusted := []string{"cloud-build", "github-actions-idp"}
		allowedRepos := []string{cmd.ImageRepository}
		if provOk, provErr := ValidateImageProvenance(cmd.Provenance, trusted, allowedRepos); !provOk {
			return false, provErr, nil
		}
	}
	return true, "", nil
}

func resolveHighestPriority(p0, p1, p2, pU int) contracts.PriorityClass {
	if p0 > 0 {
		return contracts.PriorityPrio0
	}
	if p1 > 0 {
		return contracts.PriorityPrio1
	}
	if p2 > 0 {
		return contracts.PriorityPrio2
	}
	if pU > 0 {
		return contracts.PriorityPrioU
	}
	return contracts.PriorityPrio3
}

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
			HighestPriority:  contracts.PriorityPrio0,
			Prio0Count:       len(cmd.SecretsFound),
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
			HighestPriority:  contracts.PriorityPrio0,
		}
	}

	if guardOk, guardMsg, slotViolations := evaluateSlotAndProvenanceGuards(cmd, isProd); !guardOk {
		return contracts.ImageAdmissionVerdict{
			Admitted:        false,
			ScanStatus:      contracts.ScanStatusBlocked,
			BlockReason:     guardMsg,
			SlotViolations:  slotViolations,
			HasValidSBOM:    cmd.SBOM != nil,
			HighestPriority: contracts.PriorityPrio0,
		}
	}

	criticalWithFix := 0
	criticalZeroDay := 0
	highOldFix := 0
	highCount := 0
	p0Count := 0
	p1Count := 0
	p2Count := 0
	pUCount := 0

	for _, v := range cmd.Vulnerabilities {
		prio := ClassifyVulnerabilityPriority(v, cmd.Exposure)
		switch prio {
		case contracts.PriorityPrio0:
			p0Count++
		case contracts.PriorityPrio1:
			p1Count++
		case contracts.PriorityPrio2:
			p2Count++
		case contracts.PriorityPrioU:
			pUCount++
		}

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

	highestPrio := resolveHighestPriority(p0Count, p1Count, p2Count, pUCount)

	if p0Count > 0 {
		return contracts.ImageAdmissionVerdict{
			Admitted:         false,
			ScanStatus:       contracts.ScanStatusBlocked,
			BlockReason:      fmt.Sprintf("PRIO-0 breach: %d critical active exploit or malicious package detected; waivers strictly forbidden", p0Count),
			SecretsCount:     0,
			CriticalCVECount: criticalWithFix + criticalZeroDay,
			HighCVECount:     highCount,
			HasValidSBOM:     cmd.SBOM != nil,
			WaiverApplied:    false,
			HighestPriority:  highestPrio,
			Prio0Count:       p0Count,
			Prio1Count:       p1Count,
			Prio2Count:       p2Count,
			PrioUCount:       pUCount,
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
			HighestPriority:  highestPrio,
			Prio0Count:       p0Count,
			Prio1Count:       p1Count,
			Prio2Count:       p2Count,
			PrioUCount:       pUCount,
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
			HighestPriority:  highestPrio,
			Prio0Count:       p0Count,
			Prio1Count:       p1Count,
			Prio2Count:       p2Count,
			PrioUCount:       pUCount,
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
				HighestPriority:  highestPrio,
				Prio0Count:       p0Count,
				Prio1Count:       p1Count,
				Prio2Count:       p2Count,
				PrioUCount:       pUCount,
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
			HighestPriority:  highestPrio,
			Prio0Count:       p0Count,
			Prio1Count:       p1Count,
			Prio2Count:       p2Count,
			PrioUCount:       pUCount,
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
		HighestPriority:  highestPrio,
		Prio0Count:       p0Count,
		Prio1Count:       p1Count,
		Prio2Count:       p2Count,
		PrioUCount:       pUCount,
	}
}
