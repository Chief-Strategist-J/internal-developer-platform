/*
Package contracts defines strongly typed data models, commands, records, and manifest envelopes
for the Kubernetes Workload Provisioning hexagonal feature. All inputs, records, and outputs are
strictly typecasted without raw dictionaries or untyped interfaces.
*/
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

type ManifestBundle struct {
	NamespaceYAML     string `json:"namespaceYaml"`
	ResourceQuotaYAML string `json:"resourceQuotaYaml"`
	LimitRangeYAML    string `json:"limitRangeYaml"`
	NetworkPolicyYAML string `json:"networkPolicyYaml"`
	DeploymentYAML    string `json:"deploymentYaml"`
	PDBYAML           string `json:"pdbYaml,omitempty"`
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
