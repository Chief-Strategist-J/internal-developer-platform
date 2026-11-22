package backup

import (
	"strings"

	"internal-developer-platform/src/shared/errors"
)

const (
	BackupPlanAPIVersion = "gkebackup.gke.io/v1"
	BackupPlanKind       = "BackupPlan"
	RestorePlanAPIVersion = "gkebackup.gke.io/v1"
	RestorePlanKind      = "RestorePlan"

	DefaultCronSchedule = "0 2 * * *"
	DefaultRetentionDays = 30
	MinRetentionDays     = 30

	ConflictPolicyRollbackSaveBackup = "ROLLBACK_SAVE_BACKUP"
	ConflictPolicyFailOnConflict    = "FAIL_ON_CONFLICT"

	LabelAppName        = "app.kubernetes.io/name"
	LabelManagedBy      = "app.kubernetes.io/managed-by"
	LabelManagedByValue = "internal-developer-platform"
	LabelBackupTier     = "backup.gke.io/tier"

	ErrEmptyNameMsg            = "name must not be empty"
	ErrEmptyNamespaceMsg       = "namespace must not be empty"
	ErrEmptyClusterMsg         = "cluster must not be empty"
	ErrEmptyBackupLocationMsg  = "backup location must not be empty"
	ErrInvalidRetentionDaysMsg = "retention days must be at least 30 days"
	ErrLockedRetentionRequired = "retention lock must be enabled for compliance"
	ErrSecretsRequired         = "secrets must be included in backup plan"
	ErrVolumeDataRequired      = "volume data must be included in backup plan"
)

type BackupPlanConfig struct {
	Name               string   `json:"name" yaml:"name"`
	Namespace          string   `json:"namespace" yaml:"namespace"`
	Cluster            string   `json:"cluster" yaml:"cluster"`
	Location           string   `json:"location" yaml:"location"`
	CronSchedule       string   `json:"cronSchedule" yaml:"cronSchedule"`
	RetentionDays      int      `json:"retentionDays" yaml:"retentionDays"`
	LockedRetention    bool     `json:"lockedRetention" yaml:"lockedRetention"`
	IncludeSecrets     bool     `json:"includeSecrets" yaml:"includeSecrets"`
	IncludeVolumeData  bool     `json:"includeVolumeData" yaml:"includeVolumeData"`
	TargetNamespaces   []string `json:"targetNamespaces" yaml:"targetNamespaces"`
	EncryptionKeyKMS   string   `json:"encryptionKeyKms,omitempty" yaml:"encryptionKeyKms,omitempty"`
}

type RestorePlanConfig struct {
	Name                 string   `json:"name" yaml:"name"`
	Namespace            string   `json:"namespace" yaml:"namespace"`
	TargetCluster        string   `json:"targetCluster" yaml:"targetCluster"`
	SourceBackupPlan     string   `json:"sourceBackupPlan" yaml:"sourceBackupPlan"`
	Location             string   `json:"location" yaml:"location"`
	ConflictPolicy       string   `json:"conflictPolicy" yaml:"conflictPolicy"`
	SelectedNamespaces   []string `json:"selectedNamespaces" yaml:"selectedNamespaces"`
}

func ValidateBackupPlanConfig(cfg BackupPlanConfig) error {
	if strings.TrimSpace(cfg.Name) == "" {
		return errors.NewValidationError(ErrEmptyNameMsg)
	}
	if strings.TrimSpace(cfg.Namespace) == "" {
		return errors.NewValidationError(ErrEmptyNamespaceMsg)
	}
	if strings.TrimSpace(cfg.Cluster) == "" {
		return errors.NewValidationError(ErrEmptyClusterMsg)
	}
	if strings.TrimSpace(cfg.Location) == "" {
		return errors.NewValidationError(ErrEmptyBackupLocationMsg)
	}
	if cfg.RetentionDays < MinRetentionDays {
		return errors.NewValidationError(ErrInvalidRetentionDaysMsg)
	}
	if !cfg.LockedRetention {
		return errors.NewValidationError(ErrLockedRetentionRequired)
	}
	if !cfg.IncludeSecrets {
		return errors.NewValidationError(ErrSecretsRequired)
	}
	if !cfg.IncludeVolumeData {
		return errors.NewValidationError(ErrVolumeDataRequired)
	}
	return nil
}
