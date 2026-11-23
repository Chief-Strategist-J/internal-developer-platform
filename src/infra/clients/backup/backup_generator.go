package backup

import (
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"
)

type backupPlanResource struct {
	APIVersion string                 `json:"apiVersion"`
	Kind       string                 `json:"kind"`
	Metadata   map[string]interface{} `json:"metadata"`
	Spec       backupPlanSpec         `json:"spec"`
}

type backupPlanSpec struct {
	Cluster         string          `json:"cluster"`
	RetentionPolicy retentionPolicy `json:"retentionPolicy"`
	Schedule        *scheduleSpec   `json:"schedule,omitempty"`
	BackupConfig    backupConfig    `json:"backupConfig"`
	BackupVault     string          `json:"backupVault,omitempty"`
}

type retentionPolicy struct {
	BackupDeleteLockDays int  `json:"backupDeleteLockDays"`
	BackupRetainDays     int  `json:"backupRetainDays"`
	Locked               bool `json:"locked"`
}

type scheduleSpec struct {
	CronSchedule string `json:"cronSchedule"`
	Paused       bool   `json:"paused"`
}

type backupConfig struct {
	IncludeSecrets     bool                    `json:"includeSecrets"`
	IncludeVolumeData  bool                    `json:"includeVolumeData"`
	AllNamespaces      bool                    `json:"allNamespaces,omitempty"`
	SelectedNamespaces *selectedNamespacesSpec `json:"selectedNamespaces,omitempty"`
	EncryptionKey      *encryptionKeySpec      `json:"encryptionKey,omitempty"`
}

type selectedNamespacesSpec struct {
	Namespaces []string `json:"namespaces"`
}

type encryptionKeySpec struct {
	GcpKmsEncryptionKey string `json:"gcpKmsEncryptionKey"`
}

type restorePlanResource struct {
	APIVersion string                 `json:"apiVersion"`
	Kind       string                 `json:"kind"`
	Metadata   map[string]interface{} `json:"metadata"`
	Spec       restorePlanSpec        `json:"spec"`
}

type restorePlanSpec struct {
	BackupPlan    string            `json:"backupPlan"`
	Cluster       string            `json:"cluster"`
	RestoreConfig restoreConfigSpec `json:"restoreConfig"`
}

type restoreConfigSpec struct {
	VolumeDataRestorePolicy       string                  `json:"volumeDataRestorePolicy"`
	ClusterResourceConflictPolicy string                  `json:"clusterResourceConflictPolicy"`
	NamespacedResourceRestoreMode string                  `json:"namespacedResourceRestoreMode"`
	SelectedNamespaces            *selectedNamespacesSpec `json:"selectedNamespaces,omitempty"`
}

func GenerateBackupPlanYAML(cfg BackupPlanConfig) (string, error) {
	if err := ValidateBackupPlanConfig(cfg); err != nil {
		return "", err
	}

	cron := cfg.CronSchedule
	if cron == "" {
		cron = DefaultCronSchedule
	}

	targetNs := cfg.TargetNamespaces
	if len(targetNs) == 0 {
		targetNs = []string{cfg.Namespace}
	}

	spec := backupPlanSpec{
		Cluster: cfg.Cluster,
		RetentionPolicy: retentionPolicy{
			BackupDeleteLockDays: cfg.RetentionDays,
			BackupRetainDays:     cfg.RetentionDays,
			Locked:               cfg.LockedRetention,
		},
		Schedule: &scheduleSpec{
			CronSchedule: cron,
			Paused:       false,
		},
		BackupConfig: backupConfig{
			IncludeSecrets:    cfg.IncludeSecrets,
			IncludeVolumeData: cfg.IncludeVolumeData,
			SelectedNamespaces: &selectedNamespacesSpec{
				Namespaces: targetNs,
			},
		},
	}

	if cfg.EncryptionKeyKMS != "" {
		spec.BackupConfig.EncryptionKey = &encryptionKeySpec{
			GcpKmsEncryptionKey: cfg.EncryptionKeyKMS,
		}
	}

	resource := backupPlanResource{
		APIVersion: BackupPlanAPIVersion,
		Kind:       BackupPlanKind,
		Metadata: map[string]interface{}{
			"name":      fmt.Sprintf("%s-backup-plan", cfg.Name),
			"namespace": cfg.Namespace,
			"labels": map[string]string{
				LabelAppName:    cfg.Name,
				LabelManagedBy:  LabelManagedByValue,
				LabelBackupTier: "gold-standard",
			},
		},
		Spec: spec,
	}

	bytes, err := yaml.Marshal(resource)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytes)), nil
}

func GenerateRestorePlanYAML(cfg RestorePlanConfig) (string, error) {
	if strings.TrimSpace(cfg.Name) == "" {
		return "", fmt.Errorf(ErrEmptyNameMsg)
	}
	if strings.TrimSpace(cfg.Namespace) == "" {
		return "", fmt.Errorf(ErrEmptyNamespaceMsg)
	}
	if strings.TrimSpace(cfg.TargetCluster) == "" {
		return "", fmt.Errorf(ErrEmptyClusterMsg)
	}

	conflictPolicy := cfg.ConflictPolicy
	if conflictPolicy == "" {
		conflictPolicy = ConflictPolicyRollbackSaveBackup
	}

	selectedNs := cfg.SelectedNamespaces
	if len(selectedNs) == 0 {
		selectedNs = []string{cfg.Namespace}
	}

	spec := restorePlanSpec{
		BackupPlan: cfg.SourceBackupPlan,
		Cluster:    cfg.TargetCluster,
		RestoreConfig: restoreConfigSpec{
			VolumeDataRestorePolicy:       "RESTORE_VOLUME_DATA_FROM_BACKUP",
			ClusterResourceConflictPolicy: conflictPolicy,
			NamespacedResourceRestoreMode: "FAIL_ON_CONFLICT",
			SelectedNamespaces: &selectedNamespacesSpec{
				Namespaces: selectedNs,
			},
		},
	}

	resource := restorePlanResource{
		APIVersion: RestorePlanAPIVersion,
		Kind:       RestorePlanKind,
		Metadata: map[string]interface{}{
			"name":      fmt.Sprintf("%s-restore-plan", cfg.Name),
			"namespace": cfg.Namespace,
			"labels": map[string]string{
				LabelAppName:   cfg.Name,
				LabelManagedBy: LabelManagedByValue,
			},
		},
		Spec: spec,
	}

	bytes, err := yaml.Marshal(resource)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytes)), nil
}
