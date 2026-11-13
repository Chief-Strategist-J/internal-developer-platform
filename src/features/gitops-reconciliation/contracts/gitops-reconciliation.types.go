package contracts

import "time"

type SyncPhase string

const (
	SyncPhasePending       SyncPhase = "PENDING"
	SyncPhaseSyncing       SyncPhase = "SYNCING"
	SyncPhaseSynced        SyncPhase = "SYNCED"
	SyncPhaseFailed        SyncPhase = "FAILED"
	SyncPhaseDriftDetected SyncPhase = "DRIFT_DETECTED"
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
	ID             string    `json:"id"`
	TenantID       string    `json:"tenantId"`
	ServiceID      string    `json:"serviceId"`
	AppName        string    `json:"appName"`
	TargetRevision string    `json:"targetRevision"`
	Prune          bool      `json:"prune"`
	DryRun         bool      `json:"dryRun"`
	IdempotencyKey string    `json:"idempotencyKey"`
}

type ReconciliationRecord struct {
	ID              string       `json:"id"`
	TenantID        string       `json:"tenantId"`
	ServiceID       string       `json:"serviceId"`
	AppName         string       `json:"appName"`
	TargetRevision  string       `json:"targetRevision"`
	CurrentRevision string       `json:"currentRevision"`
	SyncPhase       SyncPhase    `json:"syncPhase"`
	HealthStatus    HealthStatus `json:"healthStatus"`
	IdempotencyKey  string       `json:"idempotencyKey"`
	Version         int64        `json:"version"`
	CreatedAt       time.Time    `json:"createdAt"`
	UpdatedAt       time.Time    `json:"updatedAt"`
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

