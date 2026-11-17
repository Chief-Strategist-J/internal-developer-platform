package cluster

import (
	"time"
)

type ClusterStatus string

const (
	ClusterStatusProvisioning ClusterStatus = "PROVISIONING"
	ClusterStatusReady        ClusterStatus = "READY"
	ClusterStatusDegraded     ClusterStatus = "DEGRADED"
	ClusterStatusNonCompliant ClusterStatus = "NON_COMPLIANT"
	ClusterStatusFailed       ClusterStatus = "FAILED"
)

type ReleaseChannel string

const (
	ReleaseChannelRapid   ReleaseChannel = "RAPID"
	ReleaseChannelRegular ReleaseChannel = "REGULAR"
	ReleaseChannelStable  ReleaseChannel = "STABLE"
)

const (
	DatabaseEncryptionStateEncrypted = "ENCRYPTED"
	DatabaseEncryptionStateDecrypted = "DECRYPTED"
)

const (
	BinaryAuthModeEnforce  = "PROJECT_SINGLETON_POLICY_ENFORCE"
	BinaryAuthModeDisabled = "DISABLED"
)

const (
	MeshModeManaged = "MANAGED"
)

const (
	WorkloadPoolSuffix = ".svc.id.goog"
)

const (
	ControlIDPrivateCluster      = "GKE-SEC-01"
	ControlIDShieldedNodes       = "GKE-SEC-02"
	ControlIDWorkloadIdentity    = "GKE-SEC-03"
	ControlIDDatabaseEncryption  = "GKE-SEC-04"
	ControlIDBinaryAuthorization = "GKE-SEC-05"
	ControlIDBackupAgent         = "GKE-SEC-06"
	ControlIDReleaseChannel      = "GKE-SEC-07"
	ControlIDServiceMesh         = "GKE-SEC-08"
	ControlIDAuditLogging        = "GKE-SEC-09"
)

const (
	DescPrivateCluster      = "Private Cluster Endpoint and Private Worker Nodes"
	DescShieldedNodes       = "Shielded GKE Nodes with Secure Boot & Integrity Monitoring"
	DescWorkloadIdentity    = "Workload Identity Federation without Exported Keys"
	DescDatabaseEncryption  = "Application-Layer Secrets Encryption via Cloud KMS (CMEK)"
	DescBinaryAuthorization = "Binary Authorization Image Attestation Enforcement"
	DescBackupAgent         = "Backup for GKE Agent Provisioned"
	DescReleaseChannel      = "GKE Release Channel Auto-Upgrade Enrollment"
	DescServiceMesh         = "Managed Cloud Service Mesh Injection"
	DescAuditLogging        = "Data Access and System Audit Logging Enabled"
)

const (
	RemediationPrivateCluster      = "Enable private nodes, private endpoint, and configure master IPv4 CIDR block"
	RemediationShieldedNodes       = "Enable Shielded GKE Nodes with Secure Boot and Integrity Monitoring"
	RemediationWorkloadIdentity    = "Configure workloadPool to match project Workload Identity pool"
	RemediationDatabaseEncryption  = "Configure database encryption state to ENCRYPTED with a valid KMS key URI"
	RemediationBinaryAuthorization = "Set binary authorization evaluationMode to PROJECT_SINGLETON_POLICY_ENFORCE"
	RemediationBackupAgent         = "Enable Backup for GKE agent on cluster creation"
	RemediationReleaseChannel      = "Enroll cluster in a supported release channel (REGULAR, STABLE, RAPID)"
	RemediationServiceMesh         = "Set cloudServiceMeshMode to MANAGED"
	RemediationAuditLogging        = "Enable Data Access audit logging routed to locked retention storage"
)

type ShieldedNodesConfig struct {
	Enabled             bool `json:"enabled"`
	SecureBoot          bool `json:"secureBoot"`
	IntegrityMonitoring bool `json:"integrityMonitoring"`
}

type PrivateClusterConfig struct {
	EnablePrivateNodes    bool   `json:"enablePrivateNodes"`
	EnablePrivateEndpoint bool   `json:"enablePrivateEndpoint"`
	MasterIPv4CIDRBlock   string `json:"masterIpv4CidrBlock"`
}

type DatabaseEncryptionConfig struct {
	State   string `json:"state"`
	KeyName string `json:"keyName"`
}

type BinaryAuthorizationConfig struct {
	EvaluationMode string `json:"evaluationMode"`
}

type GKEClusterSpec struct {
	ClusterID            string                    `json:"clusterId"`
	ProjectID            string                    `json:"projectId"`
	Location             string                    `json:"location"`
	ReleaseChannel       ReleaseChannel            `json:"releaseChannel"`
	WorkloadPool         string                    `json:"workloadPool"`
	PrivateCluster       PrivateClusterConfig      `json:"privateCluster"`
	ShieldedNodes        ShieldedNodesConfig       `json:"shieldedNodes"`
	DatabaseEncryption   DatabaseEncryptionConfig  `json:"databaseEncryption"`
	BinaryAuthorization  BinaryAuthorizationConfig `json:"binaryAuthorization"`
	BackupAgentEnabled   bool                      `json:"backupAgentEnabled"`
	CloudServiceMeshMode string                    `json:"cloudServiceMeshMode"`
	DataAccessLogsActive bool                      `json:"dataAccessLogsActive"`
}

type PostureCheckResult struct {
	ControlID   string `json:"controlId"`
	Description string `json:"description"`
	Passed      bool   `json:"passed"`
	Remediation string `json:"remediation,omitempty"`
}

type ClusterPostureReport struct {
	ClusterID   string               `json:"clusterId"`
	Compliant   bool                 `json:"compliant"`
	Checks      []PostureCheckResult `json:"checks"`
	DriftCount  int                  `json:"driftCount"`
	EvaluatedAt time.Time            `json:"evaluatedAt"`
}

type GKEClusterRecord struct {
	Spec        GKEClusterSpec       `json:"spec"`
	Status      ClusterStatus        `json:"status"`
	Posture     ClusterPostureReport `json:"posture"`
	Version     int64                `json:"version"`
	CreatedAt   time.Time            `json:"createdAt"`
	UpdatedAt   time.Time            `json:"updatedAt"`
}
