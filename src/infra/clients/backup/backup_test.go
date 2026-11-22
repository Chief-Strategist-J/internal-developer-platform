package backup

import (
	"testing"
)

func TestGenerateBackupPlanYAML_Success(t *testing.T) {
	cfg := BackupPlanConfig{
		Name:              "payment-processor",
		Namespace:         "payments-prod",
		Cluster:           "projects/gov-idp-prod/locations/us-central1/clusters/gke-prod-us-central1",
		Location:          "us-central1",
		CronSchedule:      "0 2 * * *",
		RetentionDays:     30,
		LockedRetention:   true,
		IncludeSecrets:    true,
		IncludeVolumeData: true,
		TargetNamespaces:  []string{"payments-prod"},
		EncryptionKeyKMS:  "projects/gov-idp-prod/locations/us-central1/keyRings/ring/cryptoKeys/key",
	}

	yamlOut, err := GenerateBackupPlanYAML(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if yamlOut == "" {
		t.Fatal("expected non-empty yaml")
	}

	t.Logf("Generated BackupPlan YAML:\n%s", yamlOut)
}

func TestGenerateBackupPlanYAML_FloorViolations(t *testing.T) {
	baseCfg := BackupPlanConfig{
		Name:              "payment-processor",
		Namespace:         "payments-prod",
		Cluster:           "projects/gov-idp-prod/locations/us-central1/clusters/gke-prod",
		Location:          "us-central1",
		RetentionDays:     30,
		LockedRetention:   true,
		IncludeSecrets:    true,
		IncludeVolumeData: true,
	}

	t.Run("Retention days below floor", func(t *testing.T) {
		cfg := baseCfg
		cfg.RetentionDays = 14
		_, err := GenerateBackupPlanYAML(cfg)
		if err == nil {
			t.Fatal("expected error for retention days < 30")
		}
	})

	t.Run("Unlocked retention violates compliance", func(t *testing.T) {
		cfg := baseCfg
		cfg.LockedRetention = false
		_, err := GenerateBackupPlanYAML(cfg)
		if err == nil {
			t.Fatal("expected error for locked retention = false")
		}
	})

	t.Run("Missing secrets violates floor", func(t *testing.T) {
		cfg := baseCfg
		cfg.IncludeSecrets = false
		_, err := GenerateBackupPlanYAML(cfg)
		if err == nil {
			t.Fatal("expected error for include secrets = false")
		}
	})

	t.Run("Missing volume data violates floor", func(t *testing.T) {
		cfg := baseCfg
		cfg.IncludeVolumeData = false
		_, err := GenerateBackupPlanYAML(cfg)
		if err == nil {
			t.Fatal("expected error for include volume data = false")
		}
	})
}

func TestGenerateRestorePlanYAML_Success(t *testing.T) {
	cfg := RestorePlanConfig{
		Name:               "payment-processor",
		Namespace:          "payments-prod",
		TargetCluster:      "projects/gov-idp-prod/locations/us-east1/clusters/gke-standby-us-east1",
		SourceBackupPlan:   "projects/gov-idp-prod/locations/us-central1/backupPlans/payment-processor-backup-plan",
		ConflictPolicy:     ConflictPolicyRollbackSaveBackup,
		SelectedNamespaces: []string{"payments-prod"},
	}

	yamlOut, err := GenerateRestorePlanYAML(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if yamlOut == "" {
		t.Fatal("expected non-empty restore plan yaml")
	}
}
