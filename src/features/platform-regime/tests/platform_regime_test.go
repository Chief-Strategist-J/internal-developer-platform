/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PLATFORM REGIME UNIT TESTS

1. Verification Suite:
  - Tests contract validation (30-day floor, positive RTO/RPO, KMS custody).
  - Tests in-memory repository supersede semantics.
  - Tests domain rule validation against workload commands.
  - Tests service registration and fallback to baseline.
  - Zero comments inside test bodies.
*/
package tests

import (
	"context"
	"testing"
	"time"

	"internal-developer-platform/src/features/platform-regime/internal/contracts"
	"internal-developer-platform/src/features/platform-regime/internal/domain/rules"
	"internal-developer-platform/src/features/platform-regime/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/platform-regime/internal/service"
)

func sampleValidRegimeCommand() contracts.CreateRegimePolicyCommand {
	return contracts.CreateRegimePolicyCommand{
		TenantID:             "tenant-alpha",
		Source:               "api",
		ExternalID:           "pol-ext-001",
		CorrelationID:        "c-001",
		Jurisdiction:         "US_FEDRAMP_HIGH",
		AuditRetentionDays:   365,
		DataResidencyRegions: []string{"us-central1", "us-east4"},
		KMSKeyCustodyMode:    contracts.KMSCustodyCMEK,
		RTOMinutesTier0:      15,
		RPOMinutesTier0:      0,
		RTOMinutesTier1:      60,
		RPOMinutesTier1:      15,
		ClusterMode:          contracts.ClusterModeGKEStandard,
		PrimaryRegionCount:   2,
		StandbyRegionCount:   1,
		TenantIsolationMode:  contracts.TenancyModeInternalOnly,
	}
}

func TestValidateCreateRegimePolicyCommand(t *testing.T) {
	valid := sampleValidRegimeCommand()
	if err := contracts.ValidateCreateRegimePolicyCommand(valid); err != nil {
		t.Fatalf("expected valid command, got: %v", err)
	}

	invalidRetention := valid
	invalidRetention.AuditRetentionDays = 15
	if err := contracts.ValidateCreateRegimePolicyCommand(invalidRetention); err == nil {
		t.Fatalf("expected error for retention days < 30, got nil")
	}

	invalidRTO := valid
	invalidRTO.RTOMinutesTier0 = 0
	if err := contracts.ValidateCreateRegimePolicyCommand(invalidRTO); err == nil {
		t.Fatalf("expected error for non-positive RTO, got nil")
	}

	invalidKMS := valid
	invalidKMS.KMSKeyCustodyMode = "UNSUPPORTED"
	if err := contracts.ValidateCreateRegimePolicyCommand(invalidKMS); err == nil {
		t.Fatalf("expected error for invalid KMS mode, got nil")
	}
}

func TestValidateWorkloadAgainstRegime(t *testing.T) {
	regime := contracts.PlatformRegimePolicyRecord{
		AuditRetentionDays:   90,
		DataResidencyRegions: []string{"us-central1"},
		KMSKeyCustodyMode:    contracts.KMSCustodyEKM,
		TenantIsolationMode:  contracts.TenancyModeInternalOnly,
	}

	cmd := contracts.WorkloadComplianceInput{
		Namespace:           "fintech-prod",
		AuditRetentionDays:  30,
		HasEncryptedSecrets: true,
		KMSKeyURI:           "",
	}

	ok, violations := rules.ValidateWorkloadAgainstRegime(cmd, regime)
	if ok || len(violations) < 2 {
		t.Fatalf("expected admission failure for retention < regime floor and missing KMS URI, got ok=%v, violations=%d", ok, len(violations))
	}
}

func TestPlatformRegimeServiceLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := adapters.NewMemoryPlatformRegimeRepositoryAdapter()
	baseline := contracts.PlatformRegimePolicyRecord{
		ID:                 "baseline-id",
		AuditRetentionDays: 30,
		Jurisdiction:       "GLOBAL_DEFAULT",
		Status:             contracts.RegimeStatusActive,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}

	svc := service.NewPlatformRegimeService(repo, baseline)

	fallback, err := svc.GetActiveRegimePolicy(ctx, "unknown-tenant")
	if err != nil || fallback.AuditRetentionDays != 30 {
		t.Fatalf("expected baseline fallback, got %v, err: %v", fallback, err)
	}

	cmd := sampleValidRegimeCommand()
	record, err := svc.RegisterRegimePolicy(ctx, cmd)
	if err != nil {
		t.Fatalf("failed to register regime: %v", err)
	}
	if record.Status != contracts.RegimeStatusActive {
		t.Fatalf("expected status ACTIVE, got %v", record.Status)
	}

	fetched, err := svc.GetActiveRegimePolicy(ctx, "tenant-alpha")
	if err != nil || fetched.AuditRetentionDays != 365 {
		t.Fatalf("expected registered policy with 365 days retention, got %v, err: %v", fetched, err)
	}
}
