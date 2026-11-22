package tests

import (
	"context"
	"testing"
	"time"

	"internal-developer-platform/src/features/image-security/contracts"
	"internal-developer-platform/src/features/image-security/domain/machines"
	"internal-developer-platform/src/features/image-security/domain/rules"
	"internal-developer-platform/src/features/image-security/infrastructure/adapters"
	"internal-developer-platform/src/features/image-security/service"
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
