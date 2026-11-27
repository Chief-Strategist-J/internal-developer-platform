package tests

import (
	"context"
	"testing"
	"time"

	"internal-developer-platform/src/features/image-security/internal/contracts"
	"internal-developer-platform/src/features/image-security/internal/domain/machines"
	"internal-developer-platform/src/features/image-security/internal/domain/rules"
	"internal-developer-platform/src/features/image-security/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/image-security/internal/service"
)

func TestEvaluateImageSecurityPolicy_RejectsEmbeddedSecrets(t *testing.T) {
	cmd := contracts.ImageScanCommand{
		ImageDigest: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Namespace:   "payments-prod",
		TenantID:    "tenant-fintech",
		SecretsFound: []contracts.EmbeddedSecretFinding{
			{
				RuleID:      "SEC-01",
				Description: "AWS Access Key ID found in /app/.env",
				FilePath:    "/app/.env",
				SecretType:  "AWS_ACCESS_KEY",
			},
		},
	}

	verdict := rules.EvaluateImageSecurityPolicy(cmd, time.Now().UTC())
	if verdict.Admitted {
		t.Fatal("expected admission to be rejected due to embedded secrets")
	}
	if verdict.ScanStatus != contracts.ScanStatusBlocked {
		t.Fatalf("expected BLOCKED scan status, got %s", verdict.ScanStatus)
	}
}

func TestEvaluateImageSecurityPolicy_MandatesSignedSBOMInProduction(t *testing.T) {
	cmd := contracts.ImageScanCommand{
		ImageDigest: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Namespace:   "payments-prod",
		TenantID:    "tenant-fintech",
		SBOM:        nil,
	}

	verdict := rules.EvaluateImageSecurityPolicy(cmd, time.Now().UTC())
	if verdict.Admitted {
		t.Fatal("expected admission rejected in production without signed SBOM")
	}
	if verdict.HasValidSBOM {
		t.Fatal("expected HasValidSBOM to be false")
	}
}

func TestEvaluateImageSecurityPolicy_RejectsCriticalCVEWithFix(t *testing.T) {
	now := time.Now().UTC()
	cmd := contracts.ImageScanCommand{
		ImageDigest: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Namespace:   "payments-prod",
		TenantID:    "tenant-fintech",
		SBOM: &contracts.SBOMAttestation{
			Format:      "SPDX-2.3",
			Signature:   "sig-valid-cosign",
			SignerKeyID: "cosign-key-01",
		},
		Vulnerabilities: []contracts.VulnerabilityItem{
			{
				CVEID:        "CVE-2021-44228",
				PackageName:  "log4j-core",
				InstalledVer: "2.14.1",
				FixedVersion: "2.17.1",
				Severity:     contracts.SeverityCritical,
				CVSSScore:    10.0,
				HasFix:       true,
			},
		},
	}

	verdict := rules.EvaluateImageSecurityPolicy(cmd, now)
	if verdict.Admitted {
		t.Fatal("expected admission rejected due to critical CVE with available fix")
	}
	if verdict.CriticalCVECount != 1 {
		t.Fatalf("expected 1 critical CVE, got %d", verdict.CriticalCVECount)
	}
}

func TestEvaluateImageSecurityPolicy_ZeroDayWithValidWaiverPermitted(t *testing.T) {
	now := time.Now().UTC()
	cmd := contracts.ImageScanCommand{
		ImageDigest: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Namespace:   "payments-prod",
		TenantID:    "tenant-fintech",
		SBOM: &contracts.SBOMAttestation{
			Format:      "CycloneDX-1.4",
			Signature:   "sig-valid-cosign",
			SignerKeyID: "cosign-key-01",
		},
		Vulnerabilities: []contracts.VulnerabilityItem{
			{
				CVEID:        "CVE-2026-99999",
				PackageName:  "kernel-module",
				InstalledVer: "1.0.0",
				FixedVersion: "",
				Severity:     contracts.SeverityCritical,
				CVSSScore:    9.8,
				HasFix:       false,
			},
		},
		ActiveWaiver: &contracts.SecurityExceptionWaiver{
			ExceptionID:  "waiver-2026-001",
			CVEID:        "CVE-2026-99999",
			ApprovedBy:   "secops-director",
			Reason:       "Zero day patch unavailable; egress restricted",
			Compensating: "NetworkPolicy deny egress",
			ExpiresAt:    now.Add(7 * 24 * time.Hour),
		},
	}

	verdict := rules.EvaluateImageSecurityPolicy(cmd, now)
	if !verdict.Admitted {
		t.Fatalf("expected admission admitted under valid active waiver, got rejected: %s", verdict.BlockReason)
	}
	if verdict.ScanStatus != contracts.ScanStatusException {
		t.Fatalf("expected EXCEPTION_ACTIVE status, got %s", verdict.ScanStatus)
	}
	if !verdict.WaiverApplied {
		t.Fatal("expected waiver applied flag to be true")
	}
}

func TestImageSecurityStateMachine_Transitions(t *testing.T) {
	s1, err := machines.TransitionScanState(contracts.ScanStatusPending, machines.EventStartScan)
	if err != nil || s1 != contracts.ScanStatusScanning {
		t.Fatalf("expected transition to SCANNING, got %s, err: %v", s1, err)
	}

	s2, err := machines.TransitionScanState(s1, machines.EventScanPass)
	if err != nil || s2 != contracts.ScanStatusPassed {
		t.Fatalf("expected transition to PASSED, got %s, err: %v", s2, err)
	}
}

func TestImageSecurityService_AssessImageAdmissionIntegration(t *testing.T) {
	ctx := context.Background()
	scanner := adapters.NewMemoryImageScannerAdapter()
	digest := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	scanner.SetImageSBOM(digest, contracts.SBOMAttestation{
		Format:    "SPDX-2.3",
		Signature: "valid-sig",
	})
	scanner.SetImageCVEs(digest, []contracts.VulnerabilityItem{})

	svc := service.NewImageSecurityService(scanner, nil)
	cmd := contracts.ImageScanCommand{
		ImageDigest:     digest,
		ImageRepository: "registry.internal.gov/payments",
		Namespace:       "payments-prod",
		TenantID:        "tenant-fintech",
	}

	verdict, err := svc.AssessImageAdmission(ctx, cmd)
	if err != nil {
		t.Fatalf("unexpected error assessing image: %v", err)
	}
	if !verdict.Admitted {
		t.Fatalf("expected clean image to be admitted, got rejected: %s", verdict.BlockReason)
	}
}

func TestClassifyVulnerabilityPriority_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		item     contracts.VulnerabilityItem
		exposure contracts.WorkloadExposure
		expected contracts.PriorityClass
	}{
		{
			name: "backdoor is prio 0",
			item: contracts.VulnerabilityItem{
				CVEID:      "MALICIOUS-PACKAGE-001",
				IsBackdoor: true,
			},
			expected: contracts.PriorityPrio0,
		},
		{
			name: "active in the wild is prio 0",
			item: contracts.VulnerabilityItem{
				CVEID:        "CVE-2026-114938",
				Exploitation: contracts.ExploitActiveInTheWild,
				Severity:     contracts.SeverityCritical,
			},
			expected: contracts.PriorityPrio0,
		},
		{
			name: "public poc with fix is prio 1",
			item: contracts.VulnerabilityItem{
				CVEID:        "CVE-2026-123007",
				Exploitation: contracts.ExploitPublicPOC,
				FixedVersion: "1.31.9",
				Severity:     contracts.SeverityHigh,
			},
			expected: contracts.PriorityPrio1,
		},
		{
			name: "high cve with fix no exploit is prio 2",
			item: contracts.VulnerabilityItem{
				CVEID:        "CVE-2025-9999",
				FixedVersion: "2.0.0",
				Severity:     contracts.SeverityHigh,
			},
			expected: contracts.PriorityPrio2,
		},
		{
			name: "medium cve is prio 3",
			item: contracts.VulnerabilityItem{
				CVEID:    "CVE-2025-1111",
				Severity: contracts.SeverityMedium,
			},
			expected: contracts.PriorityPrio3,
		},
		{
			name: "unenriched cve is prio u",
			item: contracts.VulnerabilityItem{
				CVEID: "CVE-2026-UNKNOWN",
			},
			expected: contracts.PriorityPrioU,
		},
	}

	for _, tc := range tests {
		actual := rules.ClassifyVulnerabilityPriority(tc.item, tc.exposure)
		if actual != tc.expected {
			t.Fatalf("scenario '%s': expected %s, got %s", tc.name, tc.expected, actual)
		}
	}
}

func TestValidatePodSlotsAdmission_DSLOT(t *testing.T) {
	slots := []contracts.PodSlotImageRef{
		{
			Slot:        contracts.SlotPrimaryContainer,
			ImageName:   "app",
			ImageDigest: "repo/app@sha256:1111111111111111111111111111111111111111111111111111111111111111",
			Attested:    true,
		},
		{
			Slot:        contracts.SlotInitContainer,
			ImageName:   "migrator",
			ImageDigest: "repo/migrator@sha256:2222222222222222222222222222222222222222222222222222222222222222",
			Attested:    false,
		},
	}

	ok, violations := rules.ValidatePodSlotsAdmission(slots, true)
	if ok || len(violations) != 1 {
		t.Fatalf("expected unattested initContainer rejected in prod, got ok=%v, violations=%v", ok, violations)
	}

	unpinnedSlots := []contracts.PodSlotImageRef{
		{
			Slot:        contracts.SlotImageVolume,
			ImageName:   "config-vol",
			ImageDigest: "repo/config:latest",
			Attested:    true,
		},
	}
	okUnpinned, vUnpinned := rules.ValidatePodSlotsAdmission(unpinnedSlots, false)
	if okUnpinned || len(vUnpinned) != 1 {
		t.Fatalf("expected unpinned image volume rejected, got ok=%v, violations=%v", okUnpinned, vUnpinned)
	}
}

func TestValidateImageProvenance_B01_B02(t *testing.T) {
	trustedBuilders := []string{"cloud-build"}
	allowlist := []string{"github.com/my-org/core-api"}

	provValid := &contracts.ProvenanceAttestation{
		BuilderID:         "cloud-build",
		SourceRepo:        "github.com/my-org/core-api",
		CommitSHA:         "abcdef123456",
		IsTriggerBuild:    true,
		IsBranchProtected: true,
		IsSignedCommit:    true,
	}
	if ok, msg := rules.ValidateImageProvenance(provValid, trustedBuilders, allowlist); !ok {
		t.Fatalf("expected valid provenance to pass, got: %s", msg)
	}

	provManual := &contracts.ProvenanceAttestation{
		BuilderID:      "cloud-build",
		SourceRepo:     "github.com/my-org/core-api",
		IsTriggerBuild: false,
	}
	if ok, _ := rules.ValidateImageProvenance(provManual, trustedBuilders, allowlist); ok {
		t.Fatal("expected manual build to fail B01")
	}

	provLookalike := &contracts.ProvenanceAttestation{
		BuilderID:         "cloud-build",
		SourceRepo:        "github.com/attacker-org/core-api",
		IsTriggerBuild:    true,
		IsBranchProtected: true,
		IsSignedCommit:    true,
	}
	if ok, _ := rules.ValidateImageProvenance(provLookalike, trustedBuilders, allowlist); ok {
		t.Fatal("expected lookalike repo to fail B02")
	}
}

func TestValidateDigestVEX_D05(t *testing.T) {
	now := time.Now().UTC()
	digest := "sha256:target-digest-123"

	vexImageLevel := []contracts.VEXStatement{
		{
			VEXID:        "vex-01",
			CVEID:        "CVE-2026-9999",
			TargetDigest: "",
			Status:       "not_affected",
			ExpiresAt:    now.Add(24 * time.Hour),
		},
	}
	if ok, _ := rules.ValidateDigestVEX(vexImageLevel, digest, "CVE-2026-9999", now); ok {
		t.Fatal("expected image-level VEX to be rejected under D05")
	}

	vexValid := []contracts.VEXStatement{
		{
			VEXID:        "vex-02",
			CVEID:        "CVE-2026-9999",
			TargetDigest: digest,
			Status:       "not_affected",
			ExpiresAt:    now.Add(24 * time.Hour),
		},
	}
	if ok, msg := rules.ValidateDigestVEX(vexValid, digest, "CVE-2026-9999", now); !ok {
		t.Fatalf("expected valid digest-level VEX to pass, got: %s", msg)
	}
}
