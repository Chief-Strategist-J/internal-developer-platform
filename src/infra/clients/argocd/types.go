package argocd

import (
	"time"
)

type SyncPhase string

const (
	SyncPhaseSynced    SyncPhase = "Synced"
	SyncPhaseOutOfSync SyncPhase = "OutOfSync"
	SyncPhaseUnknown   SyncPhase = "Unknown"
)

type HealthStatusCode string

const (
	HealthStatusHealthy     HealthStatusCode = "Healthy"
	HealthStatusProgressing HealthStatusCode = "Progressing"
	HealthStatusDegraded    HealthStatusCode = "Degraded"
	HealthStatusMissing     HealthStatusCode = "Missing"
	HealthStatusUnknown     HealthStatusCode = "Unknown"
)

const (
	EndpointApplicationSync = "/api/v1/applications/%s/sync"
	EndpointApplicationGet  = "/api/v1/applications/%s"
)

type ApplicationStatusResponse struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Status struct {
		Sync struct {
			Status   SyncPhase `json:"status"`
			Revision string    `json:"revision"`
		} `json:"sync"`
		Health struct {
			Status  HealthStatusCode `json:"status"`
			Message string           `json:"message"`
		} `json:"health"`
		OperationState *struct {
			Phase      string `json:"phase"`
			Message    string `json:"message"`
			SyncResult struct {
				Resources []struct {
					Group     string `json:"group"`
					Kind      string `json:"kind"`
					Name      string `json:"name"`
					Namespace string `json:"namespace"`
					Status    string `json:"status"`
					Message   string `json:"message"`
					HookPhase string `json:"hookPhase"`
					SyncWave  int    `json:"syncWave"`
				} `json:"resources"`
			} `json:"syncResult"`
		} `json:"operationState"`
	} `json:"status"`
}

type SyncRequestPayload struct {
	Revision string `json:"revision,omitempty"`
	Prune    bool   `json:"prune"`
	DryRun   bool   `json:"dryRun"`
	Strategy *struct {
		Hook struct {
			Force bool `json:"force"`
		} `json:"hook"`
	} `json:"strategy,omitempty"`
}

type ArgoCDClientConfig struct {
	BaseURL        string
	AuthToken      string
	Timeout        time.Duration
	ConnectTimeout time.Duration
}
