package contracts

import "time"

type WorkloadStatus string

const (
	StatusSubmitted WorkloadStatus = "SUBMITTED"
	StatusValidated WorkloadStatus = "VALIDATED"
	StatusAdmitted  WorkloadStatus = "ADMITTED"
	StatusActive    WorkloadStatus = "ACTIVE"
	StatusRejected  WorkloadStatus = "REJECTED"
)

type WorkloadProvisionCommand struct {
	ID             string `json:"id"`
	TenantID       string `json:"tenantId"`
	ServiceID      string `json:"serviceId"`
	Namespace      string `json:"namespace"`
	WorkloadName   string `json:"workloadName"`
	Image          string `json:"image"`
	CPURequest     string `json:"cpuRequest"`
	CPULimit       string `json:"cpuLimit"`
	MemoryRequest  string `json:"memoryRequest"`
	MemoryLimit    string `json:"memoryLimit"`
	Replicas       int    `json:"replicas"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type WorkloadProvisionRecord struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenantId"`
	ServiceID      string         `json:"serviceId"`
	Namespace      string         `json:"namespace"`
	WorkloadName   string         `json:"workloadName"`
	CPURequest     string         `json:"cpuRequest"`
	CPULimit       string         `json:"cpuLimit"`
	MemoryRequest  string         `json:"memoryRequest"`
	MemoryLimit    string         `json:"memoryLimit"`
	Replicas       int            `json:"replicas"`
	Status         WorkloadStatus `json:"status"`
	IdempotencyKey string         `json:"idempotencyKey"`
	Version        int64          `json:"version"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
}

type WorkloadProvisionResult struct {
	Provision               WorkloadProvisionRecord `json:"provision"`
	InjectedSecurityContext bool                    `json:"injectedSecurityContext"`
	InjectedProbes          bool                    `json:"injectedProbes"`
	PDBCreated              bool                    `json:"pdbCreated"`
	Message                 string                  `json:"message"`
}

type FilterCriteria struct {
	TenantID  string
	ServiceID string
	Status    WorkloadStatus
	Limit     int
	Offset    int
}
