// Package contracts defines domain entities, commands, and DTOs for Disaster Recovery.
//
// Architectural Blueprint:
// - Step 04 Types: Pure immutable domain models, commands, results, and error types.
// - Prohibits untyped primitives; enforces strongly typed records.
// - Governed by gold-standard decisions (B-1, B-2, D-DATA, D-SEC, D-DRTOP, D-DECLARE) and 12 DR invariants.
// - 100% comment-free function bodies.
package contracts

import (
	"time"
)

type DRPlanStatus string

const (
	StatusNotReady      DRPlanStatus = "NOT_READY"
	StatusReady         DRPlanStatus = "READY"
	StatusDegraded      DRPlanStatus = "DEGRADED"
	StatusDeclared      DRPlanStatus = "DECLARED"
	StatusFencing       DRPlanStatus = "FENCING"
	StatusPromoting     DRPlanStatus = "PROMOTING"
	StatusScaling       DRPlanStatus = "SCALING"
	StatusValidating    DRPlanStatus = "VALIDATING"
	StatusRouting       DRPlanStatus = "ROUTING"
	StatusFailedOver    DRPlanStatus = "FAILED_OVER"
	StatusReconciling   DRPlanStatus = "RECONCILING"
	StatusFailbackReady DRPlanStatus = "FAILBACK_READY"
	StatusFailingBack   DRPlanStatus = "FAILING_BACK"
	StatusNormal        DRPlanStatus = "NORMAL"
	StatusAborted       DRPlanStatus = "ABORTED"
)

type DRTopology string

const (
	TopologyActiveActive DRTopology = "ACTIVE_ACTIVE"
	TopologyWarmStandby  DRTopology = "WARM_STANDBY"
	TopologyPilotLight   DRTopology = "PILOT_LIGHT"
	TopologyRestoreToNew DRTopology = "RESTORE_TO_NEW"
)

type DRDataStoreType string

const (
	DataStoreCloudSQL    DRDataStoreType = "CLOUD_SQL"
	DataStoreAlloyDB     DRDataStoreType = "ALLOYDB"
	DataStoreSpanner     DRDataStoreType = "SPANNER"
	DataStoreSelfManaged DRDataStoreType = "SELF_MANAGED"
)

type DRSecretSyncSource string

const (
	SecretSourceGoogleSecretManager DRSecretSyncSource = "GOOGLE_SECRET_MANAGER"
	SecretSourceVaultReplicated     DRSecretSyncSource = "VAULT_REPLICATED"
)

type DisasterScope string

const (
	ScopeZone       DisasterScope = "ZONE"
	ScopeRegion     DisasterScope = "REGION"
	ScopeGlobal     DisasterScope = "PROVIDER_GLOBAL"
	ScopeCorruption DisasterScope = "DATA_CORRUPTION"
	ScopeCompromise DisasterScope = "TRUST_ANCHOR_COMPROMISE"
)

type FailoverMode string

const (
	FailoverModeSwitchover FailoverMode = "SWITCHOVER"
	FailoverModeFailover   FailoverMode = "FAILOVER"
)

type DRPlanRecord struct {
	ID               string             `json:"id"`
	PublicID         string             `json:"publicId"`
	TenantID         string             `json:"tenantId"`
	Source           string             `json:"source"`
	ExternalID       string             `json:"externalId"`
	CorrelationID    string             `json:"correlationId"`
	PlanName         string             `json:"planName"`
	IncidentTier     string             `json:"incidentTier"`
	PrimaryRegion    string             `json:"primaryRegion"`
	DRRegion         string             `json:"drRegion"`
	Topology         DRTopology         `json:"topology"`
	TargetRTOSeconds int                `json:"targetRtoSeconds"`
	TargetRPOSeconds int                `json:"targetRpoSeconds"`
	Status           DRPlanStatus       `json:"status"`
	DataStoreType    DRDataStoreType    `json:"dataStoreType"`
	SecretSyncSource DRSecretSyncSource `json:"secretSyncSource"`
	PrimaryClusterID string             `json:"primaryClusterId"`
	DRClusterID      string             `json:"drClusterId"`
	LastDrillAt      *time.Time         `json:"lastDrillAt,omitempty"`
	LastDrillResult  string             `json:"lastDrillResult,omitempty"`
	IdempotencyKey   string             `json:"idempotencyKey"`
	Version          int64              `json:"version"`
	CreatedAt        time.Time          `json:"createdAt"`
	UpdatedAt        time.Time          `json:"updatedAt"`
}

type CreateDRPlanCommand struct {
	TenantID         string             `json:"tenantId"`
	Source           string             `json:"source"`
	ExternalID       string             `json:"externalId"`
	CorrelationID    string             `json:"correlationId"`
	PlanName         string             `json:"planName"`
	IncidentTier     string             `json:"incidentTier"`
	PrimaryRegion    string             `json:"primaryRegion"`
	DRRegion         string             `json:"drRegion"`
	Topology         DRTopology         `json:"topology"`
	TargetRTOSeconds int                `json:"targetRtoSeconds"`
	TargetRPOSeconds int                `json:"targetRpoSeconds"`
	DataStoreType    DRDataStoreType    `json:"dataStoreType"`
	SecretSyncSource DRSecretSyncSource `json:"secretSyncSource"`
	PrimaryClusterID string             `json:"primaryClusterId"`
	DRClusterID      string             `json:"drClusterId"`
	IdempotencyKey   string             `json:"idempotencyKey"`
}

type DependencyProbeResult struct {
	DependencyName string `json:"dependencyName"`
	Status         string `json:"status"`
	Message        string `json:"message"`
	LatencyMs      int64  `json:"latencyMs"`
}

type ReadinessCheckResult struct {
	PlanID           string                  `json:"planId"`
	OverallStatus    DRPlanStatus            `json:"overallStatus"`
	ProbeTimestamp   time.Time               `json:"probeTimestamp"`
	DependencyProbes []DependencyProbeResult `json:"dependencyProbes"`
}

type DRIncidentRecord struct {
	ID             string        `json:"id"`
	PublicID       string        `json:"publicId"`
	TenantID       string        `json:"tenantId"`
	PlanID         string        `json:"planId"`
	Scope          DisasterScope `json:"scope"`
	DeclaredBy     string        `json:"declaredBy"`
	SecondApprover string        `json:"secondApprover,omitempty"`
	EvidenceLinks  []string      `json:"evidenceLinks"`
	State          string        `json:"state"`
	CreatedAt      time.Time     `json:"createdAt"`
}

type DeclareIncidentCommand struct {
	TenantID      string        `json:"tenantId"`
	PlanID        string        `json:"planId"`
	Scope         DisasterScope `json:"scope"`
	Declarer      string        `json:"declarer"`
	EvidenceLinks []string      `json:"evidenceLinks"`
}

type ApproveIncidentCommand struct {
	TenantID   string `json:"tenantId"`
	IncidentID string `json:"incidentId"`
	Approver   string `json:"approver"`
}

type DRFailoverOperationRecord struct {
	ID           string            `json:"id"`
	PublicID     string            `json:"publicId"`
	TenantID     string            `json:"tenantId"`
	PlanID       string            `json:"planId"`
	IncidentID   string            `json:"incidentId"`
	Mode         FailoverMode      `json:"mode"`
	Initiator    string            `json:"initiator"`
	Approver     string            `json:"approver,omitempty"`
	State        DRPlanStatus      `json:"state"`
	ProjectedRPO int               `json:"projectedRpo"`
	MeasuredRTO  int               `json:"measuredRto"`
	StepResults  map[string]string `json:"stepResults"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

type InitiateFailoverCommand struct {
	TenantID   string       `json:"tenantId"`
	PlanID     string       `json:"planId"`
	IncidentID string       `json:"incidentId"`
	Mode       FailoverMode `json:"mode"`
	Initiator  string       `json:"initiator"`
}

type ApproveFailoverCommand struct {
	TenantID    string `json:"tenantId"`
	OperationID string `json:"operationId"`
	Approver    string `json:"approver"`
}

type AbortFailoverCommand struct {
	TenantID    string `json:"tenantId"`
	OperationID string `json:"operationId"`
	Actor       string `json:"actor"`
	Reason      string `json:"reason"`
}

type FencingEpochRecord struct {
	DatasetID string    `json:"datasetId"`
	Authority string    `json:"authority"`
	Epoch     int64     `json:"epoch"`
	FencedAt  time.Time `json:"fencedAt"`
}

type FenceDatasetCommand struct {
	TenantID  string `json:"tenantId"`
	DatasetID string `json:"datasetId"`
	Authority string `json:"authority"`
	Actor     string `json:"actor"`
}

type ExecuteFailbackCommand struct {
	TenantID   string `json:"tenantId"`
	PlanID     string `json:"planId"`
	Initiator  string `json:"initiator"`
	Approver   string `json:"approver"`
	Divergence int    `json:"divergence"`
	ReverseLag int    `json:"reverseLag"`
}

type DRFailbackOperationRecord struct {
	ID          string            `json:"id"`
	PublicID    string            `json:"publicId"`
	TenantID    string            `json:"tenantId"`
	PlanID      string            `json:"planId"`
	Initiator   string            `json:"initiator"`
	Approver    string            `json:"approver"`
	State       DRPlanStatus      `json:"state"`
	StepResults map[string]string `json:"stepResults"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

type RunDrillCommand struct {
	TenantID      string `json:"tenantId"`
	PlanID        string `json:"planId"`
	Scope         string `json:"scope"`
	Initiator     string `json:"initiator"`
	SyntheticOnly bool   `json:"syntheticOnly"`
}

type DrillRunRecord struct {
	ID               string    `json:"id"`
	PublicID         string    `json:"publicId"`
	TenantID         string    `json:"tenantId"`
	PlanID           string    `json:"planId"`
	Scope            string    `json:"scope"`
	MeasuredRTOSecs  int       `json:"measuredRtoSecs"`
	MeasuredRPOSecs  int       `json:"measuredRpoSecs"`
	TargetRTOSecs    int       `json:"targetRtoSecs"`
	TargetRPOSecs    int       `json:"targetRpoSecs"`
	Passed           bool      `json:"passed"`
	EvidenceBundleID string    `json:"evidenceBundleId"`
	ExecutedAt       time.Time `json:"executedAt"`
}

type InitiateRestoreCommand struct {
	TenantID       string `json:"tenantId"`
	PlanID         string `json:"planId"`
	BackupID       string `json:"backupId"`
	TargetCluster  string `json:"targetCluster"`
	Namespace      string `json:"namespace"`
	ConflictPolicy string `json:"conflictPolicy"`
	Initiator      string `json:"initiator"`
}

type DRRestoreOperationRecord struct {
	ID             string    `json:"id"`
	PublicID       string    `json:"publicId"`
	TenantID       string    `json:"tenantId"`
	PlanID         string    `json:"planId"`
	BackupID       string    `json:"backupId"`
	TargetCluster  string    `json:"targetCluster"`
	Namespace      string    `json:"namespace"`
	State          string    `json:"state"`
	DataValidated  bool      `json:"dataValidated"`
	ValidationNote string    `json:"validationNote"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type ActivateBreakglassCommand struct {
	TenantID      string `json:"tenantId"`
	Requester     string `json:"requester"`
	Approver      string `json:"approver"`
	DurationMins  int    `json:"durationMins"`
	Justification string `json:"justification"`
}

type BreakglassGrantRecord struct {
	ID            string    `json:"id"`
	PublicID      string    `json:"publicId"`
	TenantID      string    `json:"tenantId"`
	Requester     string    `json:"requester"`
	Approver      string    `json:"approver"`
	Role          string    `json:"role"`
	ExpiresAt     time.Time `json:"expiresAt"`
	Justification string    `json:"justification"`
	Active        bool      `json:"active"`
	CreatedAt     time.Time `json:"createdAt"`
}

type EvidenceBundleRecord struct {
	ID               string    `json:"id"`
	PublicID         string    `json:"publicId"`
	TenantID         string    `json:"tenantId"`
	DrillRunID       string    `json:"drillRunId"`
	PlanID           string    `json:"planId"`
	ExecutedBy       string    `json:"executedBy"`
	Scope            string    `json:"scope"`
	MeasuredRTOSecs  int       `json:"measuredRtoSecs"`
	MeasuredRPOSecs  int       `json:"measuredRpoSecs"`
	Passed           bool      `json:"passed"`
	VerificationLogs []string  `json:"verificationLogs"`
	AuditSinkRegion  string    `json:"auditSinkRegion"`
	ChecksumSha256   string    `json:"checksumSha256"`
	CreatedAt        time.Time `json:"createdAt"`
}

type LiveRPOResult struct {
	PlanID           string    `json:"planId"`
	PlanName         string    `json:"planName"`
	DataStoreType    string    `json:"dataStoreType"`
	PrimaryRegion    string    `json:"primaryRegion"`
	DRRegion         string    `json:"drRegion"`
	CurrentLagSecs   int       `json:"currentLagSecs"`
	TargetRPOSeconds int       `json:"targetRpoSecs"`
	WithinSLA        bool      `json:"withinSla"`
	CheckedAt        time.Time `json:"checkedAt"`
}
