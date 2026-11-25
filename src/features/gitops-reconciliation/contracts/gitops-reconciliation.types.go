package contracts

import "time"

type SyncPhase string

const (
	SyncPhasePending       SyncPhase = "PENDING"
	SyncPhaseSyncing       SyncPhase = "SYNCING"
	SyncPhaseSynced        SyncPhase = "SYNCED"
	SyncPhaseFailed        SyncPhase = "FAILED"
	SyncPhaseDriftDetected SyncPhase = "DRIFT_DETECTED"
	SyncPhaseRollingBack   SyncPhase = "ROLLING_BACK"
)

type HealthStatus string

const (
	HealthStatusHealthy     HealthStatus = "HEALTHY"
	HealthStatusProgressing HealthStatus = "PROGRESSING"
	HealthStatusDegraded    HealthStatus = "DEGRADED"
	HealthStatusMissing     HealthStatus = "MISSING"
	HealthStatusUnknown     HealthStatus = "UNKNOWN"
)

type DriftClassificationType string

const (
	DriftTypeAModified   DriftClassificationType = "TYPE_A_MODIFIED"
	DriftTypeBMissing    DriftClassificationType = "TYPE_B_MISSING"
	DriftTypeCExtraneous DriftClassificationType = "TYPE_C_EXTRANEOUS"
	DriftTypeDOrphaned   DriftClassificationType = "TYPE_D_ORPHANED"
)

type SyncCommand struct {
	ID                    string `json:"id"`
	TenantID              string `json:"tenantId"`
	ServiceID             string `json:"serviceId"`
	AppName               string `json:"appName"`
	TargetRevision        string `json:"targetRevision"`
	LastKnownGoodRevision string `json:"lastKnownGoodRevision,omitempty"`
	Prune                 bool   `json:"prune"`
	DryRun                bool   `json:"dryRun"`
	IdempotencyKey        string `json:"idempotencyKey"`
}

type RollbackResult struct {
	PreviousRevision string `json:"previousRevision"`
	RestoredRevision string `json:"restoredRevision"`
	Reason           string `json:"reason"`
	Success          bool   `json:"success"`
}

type ReconciliationRecord struct {
	ID                    string          `json:"id"`
	TenantID              string          `json:"tenantId"`
	ServiceID             string          `json:"serviceId"`
	AppName               string          `json:"appName"`
	TargetRevision        string          `json:"targetRevision"`
	CurrentRevision       string          `json:"currentRevision"`
	LastKnownGoodRevision string          `json:"lastKnownGoodRevision,omitempty"`
	SyncPhase             SyncPhase       `json:"syncPhase"`
	HealthStatus          HealthStatus    `json:"healthStatus"`
	RollbackTriggered     bool            `json:"rollbackTriggered"`
	RollbackDetails       *RollbackResult `json:"rollbackDetails,omitempty"`
	IdempotencyKey        string          `json:"idempotencyKey"`
	Version               int64           `json:"version"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
}

type SyncResult struct {
	Reconciliation ReconciliationRecord `json:"reconciliation"`
	AppliedHooks   int                  `json:"appliedHooks"`
	Message        string               `json:"message"`
}

type FilterCriteria struct {
	TenantID  string
	ServiceID string
	SyncPhase SyncPhase
	Limit     int
	Offset    int
}

type GitOpsWebhookPayload struct {
	RepositoryURL string `json:"repositoryUrl"`
	Branch        string `json:"branch"`
	CommitSHA     string `json:"commitSha"`
	Author        string `json:"author"`
	EventType     string `json:"eventType"`
}

type GitOpsWebhookResult struct {
	Accepted       bool     `json:"accepted"`
	RefreshedApps  []string `json:"refreshedApps"`
	TriggeredSyncs []string `json:"triggeredSyncs"`
	Message        string   `json:"message"`
}

type ClassifiedDriftItem struct {
	ResourceKind     string                  `json:"resourceKind"`
	ResourceName     string                  `json:"resourceName"`
	Namespace        string                  `json:"namespace"`
	Classification   DriftClassificationType `json:"classification"`
	LiveValue        string                  `json:"liveValue,omitempty"`
	DesiredValue     string                  `json:"desiredValue,omitempty"`
	FieldPath        string                  `json:"fieldPath,omitempty"`
	IsRuntimeIgnored bool                    `json:"isRuntimeIgnored"`
}

type DetailedDriftReport struct {
	AppName         string                `json:"appName"`
	TargetRevision  string                `json:"targetRevision"`
	HasDrift        bool                  `json:"hasDrift"`
	Items           []ClassifiedDriftItem `json:"items"`
	TotalDriftCount int                   `json:"totalDriftCount"`
	IgnoredCount    int                   `json:"ignoredCount"`
	CanAutoSync     bool                  `json:"canAutoSync"`
}

type SecretMechanismType string

const (
	SecretMechanismSealedSecrets           SecretMechanismType = "SEALED_SECRETS"
	SecretMechanismExternalSecretsOperator SecretMechanismType = "EXTERNAL_SECRETS_OPERATOR"
	SecretMechanismSecretsStoreCSI         SecretMechanismType = "SECRETS_STORE_CSI_DRIVER"
)

type SecretBackendConfig struct {
	TenantID         string              `json:"tenantId"`
	Mechanism        SecretMechanismType `json:"mechanism"`
	VaultEndpoint    string              `json:"vaultEndpoint,omitempty"`
	ClusterScope     string              `json:"clusterScope,omitempty"`
	SecretStoreRef   string              `json:"secretStoreRef,omitempty"`
	CSIDriverName    string              `json:"csiDriverName,omitempty"`
	EncryptionKeyRef string              `json:"encryptionKeyRef,omitempty"`
	Enabled          bool                `json:"enabled"`
}

type SecretReference struct {
	Name          string              `json:"name"`
	Namespace     string              `json:"namespace"`
	Mechanism     SecretMechanismType `json:"mechanism"`
	RemoteKeyRef  string              `json:"remoteKeyRef,omitempty"`
	EncryptedData map[string]string   `json:"encryptedData,omitempty"`
}

type SecretSyncValidationResult struct {
	Allowed       bool                `json:"allowed"`
	Mechanism     SecretMechanismType `json:"mechanism"`
	Reason        string              `json:"reason"`
	ViolationCode string              `json:"violationCode,omitempty"`
}
