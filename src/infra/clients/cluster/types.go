package cluster

import (
	"time"
)

type ClusterStatus string

const (
	ClusterStatusProvisioning ClusterStatus = "PROVISIONING"
	ClusterStatusReady        ClusterStatus = "READY"
	ClusterStatusDegraded     ClusterStatus = "DEGRADED"
	ClusterStatusNonCompliant  ClusterStatus = "NON_COMPLIANT"
	ClusterStatusFailed       ClusterStatus = "FAILED"
)

type ReleaseChannel string

const (
	ReleaseChannelRapid   ReleaseChannel = "RAPID"
	ReleaseChannelRegular ReleaseChannel = "REGULAR"
	ReleaseChannelStable  ReleaseChannel = "STABLE"
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
