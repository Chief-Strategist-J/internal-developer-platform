package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

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
	StatusDecommissioning WorkloadStatus = "DECOMMISSIONING"
	StatusDecommissioned  WorkloadStatus = "DECOMMISSIONED"
)

type DeploymentStrategy string

const (
	StrategyRollingUpdate DeploymentStrategy = "RollingUpdate"
	StrategyRecreate      DeploymentStrategy = "Recreate"
	StrategyCanary        DeploymentStrategy = "Canary"
)

type IncidentTier string

const (
	IncidentTier0 IncidentTier = "TIER_0"
	IncidentTier1 IncidentTier = "TIER_1"
	IncidentTier2 IncidentTier = "TIER_2"
	IncidentTier3 IncidentTier = "TIER_3"
)

type IncidentTimelineEvent struct {
	ID          string                 `json:"id"`
	TenantID    string                 `json:"tenantId"`
	WorkloadID  string                 `json:"workloadId"`
	EventType   string                 `json:"eventType"`
	Description string                 `json:"description"`
	Actor       string                 `json:"actor"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"createdAt"`
}

type CanaryStep struct {
	SetWeight            int `json:"setWeight"`
	PauseDurationSeconds int `json:"pauseDurationSeconds,omitempty"`
}

type AnalysisMetric struct {
	Name                  string `json:"name"`
	Interval              string `json:"interval"`
	Count                 int    `json:"count,omitempty"`
	SuccessCondition      string `json:"successCondition"`
	FailureLimit          int    `json:"failureLimit"`
	InconclusiveLimit     int    `json:"inconclusiveLimit"`
	ConsecutiveErrorLimit int    `json:"consecutiveErrorLimit"`
	Query                 string `json:"query"`
}

type RolloutConfig struct {
	Strategy        DeploymentStrategy `json:"strategy"`
	CanarySteps     []CanaryStep       `json:"canarySteps,omitempty"`
	AnalysisMetrics []AnalysisMetric   `json:"analysisMetrics,omitempty"`
}

type MeshSecurityConfig struct {
	MtlsMode          string   `json:"mtlsMode"`
	AllowedPrincipals []string `json:"allowedPrincipals,omitempty"`
	AllowedMethods    []string `json:"allowedMethods,omitempty"`
}

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

type EgressRule struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type AuditLoggingConfig struct {
	EnableDataAccessLogs bool   `json:"enableDataAccessLogs"`
	DestinationBucket    string `json:"destinationBucket"`
	RetentionDays        int    `json:"retentionDays"`
	LockedRetention      bool   `json:"lockedRetention"`
	KMSKeyURI            string `json:"kmsKeyUri,omitempty"`
	OffNodeLogCollection bool   `json:"offNodeLogCollection"`
	PIIScrubbingEnabled  bool   `json:"piiScrubbingEnabled"`
}

type SecretVolumeMount struct {
	VolumeName string `json:"volumeName"`
	SecretName string `json:"secretName"`
	MountPath  string `json:"mountPath"`
	SubPath    string `json:"subPath,omitempty"`
	ReadOnly   bool   `json:"readOnly,omitempty"`
}

type CertificateExpiryStatus struct {
	NeedsRenewal   bool      `json:"needsRenewal"`
	DaysRemaining  int       `json:"daysRemaining"`
	AlertSeverity  string    `json:"alertSeverity"`
	ExpirationTime time.Time `json:"expirationTime"`
}

type BackupPlanConfig struct {
	Enabled           bool     `json:"enabled"`
	Cluster           string   `json:"cluster"`
	Location          string   `json:"location"`
	CronSchedule      string   `json:"cronSchedule,omitempty"`
	RetentionDays     int      `json:"retentionDays"`
	LockedRetention   bool     `json:"lockedRetention"`
	IncludeSecrets    bool     `json:"includeSecrets"`
	IncludeVolumeData bool     `json:"includeVolumeData"`
	EncryptionKeyKMS  string   `json:"encryptionKeyKms,omitempty"`
}

type RestorePlanConfig struct {
	Enabled          bool   `json:"enabled"`
	TargetCluster    string `json:"targetCluster"`
	SourceBackupPlan string `json:"sourceBackupPlan"`
	ConflictPolicy   string `json:"conflictPolicy,omitempty"`
}

type DecommissionDecision struct {
	ApprovedBy       string    `json:"approvedBy"`
	Reason           string    `json:"reason"`
	RetentionDays    int       `json:"retentionDays"`
	LockBackups      bool      `json:"lockBackups"`
	DecisionRecorded bool      `json:"decisionRecorded"`
	RecordedAt       time.Time `json:"recordedAt"`
}

type WorkloadDecommissionCommand struct {
	TenantID     string               `json:"tenantId"`
	Namespace    string               `json:"namespace"`
	WorkloadName string               `json:"workloadName"`
	Decision     DecommissionDecision `json:"decision"`
}

type DecommissionExecutionStep string

const (
	StepDecommissionDrainTraffic      DecommissionExecutionStep = "DRAIN_TRAFFIC"
	StepDecommissionRemoveWorkload    DecommissionExecutionStep = "REMOVE_WORKLOAD"
	StepDecommissionRemoveBindings    DecommissionExecutionStep = "REMOVE_BINDINGS"
	StepDecommissionReleaseStorage    DecommissionExecutionStep = "RELEASE_STORAGE"
	StepDecommissionDeprovisionIdentity DecommissionExecutionStep = "DEPROVISION_IDENTITY"
	StepDecommissionRemoveDNS         DecommissionExecutionStep = "REMOVE_DNS"
	StepDecommissionRecordAudit       DecommissionExecutionStep = "RECORD_AUDIT"
)

type DecommissionExecutionResult struct {
	WorkloadName string                      `json:"workloadName"`
	Namespace    string                      `json:"namespace"`
	Status       WorkloadStatus              `json:"status"`
	StepsExecuted []DecommissionExecutionStep `json:"stepsExecuted"`
	AuditRecordID string                     `json:"auditRecordId"`
}

type WorkloadProvisionCommand struct {
	ID               string             `json:"id"`
	PublicID         string             `json:"publicId,omitempty"`
	TenantID         string             `json:"tenantId"`
	Source           string             `json:"source,omitempty"`
	ExternalID       string             `json:"externalId,omitempty"`
	CorrelationID    string             `json:"correlationId,omitempty"`
	ServiceID        string             `json:"serviceId"`
	Namespace        string             `json:"namespace"`
	WorkloadName     string             `json:"workloadName"`
	Image            string             `json:"image"`
	ImagePullPolicy  string             `json:"imagePullPolicy,omitempty"`
	BreakglassJustification string      `json:"breakglassJustification,omitempty"`
	CPURequest       string             `json:"cpuRequest"`
	CPULimit         string             `json:"cpuLimit"`
	MemoryRequest    string             `json:"memoryRequest"`
	MemoryLimit      string             `json:"memoryLimit"`
	Replicas         int                `json:"replicas"`
	Strategy         DeploymentStrategy `json:"strategy,omitempty"`
	IdempotencyKey   string             `json:"idempotencyKey"`
	ApprovedSpecHash string             `json:"approvedSpecHash,omitempty"`
	ApproverRole     string             `json:"approverRole,omitempty"`
	SecretMounts     []SecretVolumeMount `json:"secretMounts,omitempty"`
	KMSKeyURI        string              `json:"kmsKeyUri,omitempty"`
	EncryptedSecrets map[string]string   `json:"encryptedSecrets,omitempty"`
	TLSHosts         []string            `json:"tlsHosts,omitempty"`
	MeshSecurity     MeshSecurityConfig  `json:"meshSecurity,omitempty"`
	UnsupportedMeshResources []string    `json:"unsupportedMeshResources,omitempty"`
	EgressRules      []EgressRule        `json:"egressRules,omitempty"`
	AuditLogging     AuditLoggingConfig  `json:"auditLogging,omitempty"`
	BackupPlan       BackupPlanConfig    `json:"backupPlan,omitempty"`
	RestorePlan      RestorePlanConfig   `json:"restorePlan,omitempty"`
	OnCallPager      string              `json:"onCallPager,omitempty"`
	RunbookURL       string              `json:"runbookUrl,omitempty"`
	IncidentTier     IncidentTier        `json:"incidentTier,omitempty"`
	RequestedResources []string          `json:"requestedResources,omitempty"`
}

type WorkloadProvisionRecord struct {
	ID               string             `json:"id"`
	PublicID         string             `json:"publicId"`
	TenantID         string             `json:"tenantId"`
	Source           string             `json:"source"`
	ExternalID       string             `json:"externalId"`
	CorrelationID    string             `json:"correlationId"`
	SpecHash         string             `json:"specHash"`
	ApprovedSpecHash string             `json:"approvedSpecHash"`
	ApproverRole     string             `json:"approverRole"`
	ImagePullPolicy  string             `json:"imagePullPolicy"`
	BreakglassJustification string      `json:"breakglassJustification"`
	ServiceID        string             `json:"serviceId"`
	Namespace        string             `json:"namespace"`
	WorkloadName     string             `json:"workloadName"`
	CPURequest       string             `json:"cpuRequest"`
	CPULimit         string             `json:"cpuLimit"`
	MemoryRequest    string             `json:"memoryRequest"`
	MemoryLimit      string             `json:"memoryLimit"`
	Replicas         int                `json:"replicas"`
	Strategy         DeploymentStrategy `json:"strategy"`
	Status           WorkloadStatus     `json:"status"`
	EnableDataAccessLogs   bool         `json:"enableDataAccessLogs"`
	AuditDestinationBucket string       `json:"auditDestinationBucket"`
	AuditRetentionDays     int          `json:"auditRetentionDays"`
	AuditLockedRetention   bool         `json:"auditLockedRetention"`
	OnCallPager      string             `json:"onCallPager"`
	RunbookURL       string             `json:"runbookUrl"`
	IncidentTier     IncidentTier       `json:"incidentTier"`
	IdempotencyKey   string             `json:"idempotencyKey"`
	Version          int64              `json:"version"`
	CreatedAt        time.Time          `json:"createdAt"`
	UpdatedAt        time.Time          `json:"updatedAt"`
}

func ComputeWorkloadSpecHash(cmd WorkloadProvisionCommand) string {
	strategy := cmd.Strategy
	if strategy == "" {
		strategy = StrategyCanary
	}
	canonical := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%d|%s",
		strings.TrimSpace(cmd.TenantID),
		strings.TrimSpace(cmd.ServiceID),
		strings.TrimSpace(cmd.Namespace),
		strings.TrimSpace(cmd.WorkloadName),
		strings.TrimSpace(cmd.Image),
		strings.TrimSpace(cmd.CPURequest),
		strings.TrimSpace(cmd.CPULimit),
		strings.TrimSpace(cmd.MemoryRequest),
		strings.TrimSpace(cmd.MemoryLimit),
		cmd.Replicas,
		strategy,
	)
	hasher := sha256.New()
	hasher.Write([]byte(canonical))
	return hex.EncodeToString(hasher.Sum(nil))
}

type ManifestBundle struct {
	NamespaceYAML           string `json:"namespaceYaml"`
	ResourceQuotaYAML       string `json:"resourceQuotaYaml"`
	LimitRangeYAML          string `json:"limitRangeYaml"`
	NetworkPolicyYAML       string `json:"networkPolicyYaml"`
	DeploymentYAML          string `json:"deploymentYaml"`
	PDBYAML                 string `json:"pdbYaml,omitempty"`
	ArgoCDApplicationYAML   string `json:"argocdApplicationYaml,omitempty"`
	RolloutYAML             string `json:"rolloutYaml,omitempty"`
	AnalysisTemplateYAML    string `json:"analysisTemplateYaml,omitempty"`
	AuthorizationPolicyYAML string `json:"authorizationPolicyYaml,omitempty"`
	VirtualServiceYAML      string `json:"virtualServiceYaml,omitempty"`
	DestinationRuleYAML     string `json:"destinationRuleYaml,omitempty"`
	WorkloadIdentityYAML    string `json:"workloadIdentityYaml,omitempty"`
	DatabaseYAML            string `json:"databaseYaml,omitempty"`
	HTTPRouteYAML           string `json:"httpRouteYaml,omitempty"`
	BackendPolicyYAML       string `json:"backendPolicyYaml,omitempty"`
	SecurityPolicyYAML      string `json:"securityPolicyYaml,omitempty"`
	CertificateYAML         string `json:"certificateYaml,omitempty"`
	SecretYAML              string `json:"secretYaml,omitempty"`
	PeerAuthenticationYAML  string `json:"peerAuthenticationYaml,omitempty"`
	ServiceEntryYAML        string `json:"serviceEntryYaml,omitempty"`
	LogSinkYAML             string `json:"logSinkYaml,omitempty"`
	LoggingBucketYAML       string `json:"loggingBucketYaml,omitempty"`
	BackupPlanYAML          string `json:"backupPlanYaml,omitempty"`
	RestorePlanYAML         string `json:"restorePlanYaml,omitempty"`
}

type AdmissionViolation struct {
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

type AdmissionReport struct {
	Admitted         bool                 `json:"admitted"`
	Violations       []AdmissionViolation `json:"violations,omitempty"`
	ExceptionApplied bool                 `json:"exceptionApplied,omitempty"`
	ExceptionID      string               `json:"exceptionId,omitempty"`
	ApproverID       string               `json:"approverId,omitempty"`
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
