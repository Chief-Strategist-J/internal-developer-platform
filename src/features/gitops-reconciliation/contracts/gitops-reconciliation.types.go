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
