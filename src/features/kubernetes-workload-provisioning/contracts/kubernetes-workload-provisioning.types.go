package contracts

import "time"

type WorkloadStatus string

const (
	StatusSubmitted     WorkloadStatus = "SUBMITTED"
	StatusValidated     WorkloadStatus = "VALIDATED"
	StatusAdmitted      WorkloadStatus = "ADMITTED"
	StatusProvisioning  WorkloadStatus = "PROVISIONING"
	StatusActive        WorkloadStatus = "ACTIVE"
	StatusObservedReady WorkloadStatus = "OBSERVED_READY"
	StatusDegraded      WorkloadStatus = "DEGRADED"
	StatusRejected      WorkloadStatus = "REJECTED"
)

type ResourceQuotaSpec struct {
	HardCPU    string `json:"hardCpu"`
	HardMemory string `json:"hardMemory"`
	UsedCPU    string `json:"usedCpu"`
	UsedMemory string `json:"usedMemory"`
}

type QuotaCheckResult struct {
	SufficientHeadroom bool   `json:"sufficientHeadroom"`
	AvailableCPU       string `json:"availableCpu"`
	AvailableMemory    string `json:"availableMemory"`
	ViolationMessage   string `json:"violationMessage,omitempty"`
}

type GitOpsAppConfig struct {
	ProjectName    string `json:"projectName"`
	RepoURL        string `json:"repoUrl"`
	TargetRevision string `json:"targetRevision"`
	Path           string `json:"path"`
	ServerURL      string `json:"serverUrl"`
}

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

type ManifestBundle struct {
	NamespaceYAML         string `json:"namespaceYaml"`
	ResourceQuotaYAML     string `json:"resourceQuotaYaml"`
	LimitRangeYAML        string `json:"limitRangeYaml"`
	NetworkPolicyYAML     string `json:"networkPolicyYaml"`
	DeploymentYAML        string `json:"deploymentYaml"`
	PDBYAML               string `json:"pdbYaml,omitempty"`
	ArgoCDApplicationYAML string `json:"argocdApplicationYaml,omitempty"`
}

type AdmissionViolation struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

type AdmissionReport struct {
	Admitted   bool                 `json:"admitted"`
	Violations []AdmissionViolation `json:"violations,omitempty"`
}

type WorkloadProvisionResult struct {
	Provision               WorkloadProvisionRecord `json:"provision"`
	InjectedSecurityContext bool                    `json:"injectedSecurityContext"`
	InjectedProbes          bool                    `json:"injectedProbes"`
	PDBCreated              bool                    `json:"pdbCreated"`
	Manifests               ManifestBundle          `json:"manifests"`
	Admission               AdmissionReport         `json:"admission"`
	Message                 string                  `json:"message"`
}

type FilterCriteria struct {
	TenantID  string
	ServiceID string
	Status    WorkloadStatus
	Limit     int
	Offset    int
}
