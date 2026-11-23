package contracts

import (
	"time"
)

type VulnerabilitySeverity string

const (
	SeverityCritical VulnerabilitySeverity = "CRITICAL"
	SeverityHigh     VulnerabilitySeverity = "HIGH"
	SeverityMedium   VulnerabilitySeverity = "MEDIUM"
	SeverityLow      VulnerabilitySeverity = "LOW"
)

type ScanStatus string

const (
	ScanStatusPending   ScanStatus = "PENDING"
	ScanStatusScanning  ScanStatus = "SCANNING"
	ScanStatusPassed    ScanStatus = "PASSED"
	ScanStatusBlocked   ScanStatus = "BLOCKED"
	ScanStatusException ScanStatus = "EXCEPTION_ACTIVE"
)

type PriorityClass string

const (
	PriorityPrio0 PriorityClass = "PRIO_0"
	PriorityPrio1 PriorityClass = "PRIO_1"
	PriorityPrio2 PriorityClass = "PRIO_2"
	PriorityPrio3 PriorityClass = "PRIO_3"
	PriorityPrioU PriorityClass = "PRIO_U"
)

type ExploitationEvidence string

const (
	ExploitActiveInTheWild ExploitationEvidence = "ACTIVE_IN_THE_WILD"
	ExploitPublicPOC       ExploitationEvidence = "PUBLIC_POC"
	ExploitReportedVendor  ExploitationEvidence = "REPORTED_VENDOR"
	ExploitNone            ExploitationEvidence = "NONE"
)

type WorkloadExposure string

const (
	ExposureInternetFacing WorkloadExposure = "INTERNET_FACING"
	ExposureInternalOnly   WorkloadExposure = "INTERNAL_ONLY"
	ExposurePrivileged     WorkloadExposure = "PRIVILEGED"
)

type PodContainerSlot string

const (
	SlotPrimaryContainer PodContainerSlot = "CONTAINER"
	SlotInitContainer    PodContainerSlot = "INIT_CONTAINER"
	SlotNativeSidecar    PodContainerSlot = "NATIVE_SIDECAR"
	SlotImageVolume      PodContainerSlot = "IMAGE_VOLUME"
	SlotEphemeralDebug   PodContainerSlot = "EPHEMERAL_DEBUG"
)

type PodSlotImageRef struct {
	Slot        PodContainerSlot `json:"slot"`
	ImageName   string           `json:"imageName"`
	ImageDigest string           `json:"imageDigest"`
	Attested    bool             `json:"attested"`
}

type ProvenanceAttestation struct {
	BuilderID         string `json:"builderId"`
	SourceRepo        string `json:"sourceRepo"`
	CommitSHA         string `json:"commitSha"`
	IsTriggerBuild    bool   `json:"isTriggerBuild"`
	IsBranchProtected bool   `json:"isBranchProtected"`
	IsSignedCommit    bool   `json:"isSignedCommit"`
}

type VEXStatement struct {
	VEXID         string    `json:"vexId"`
	CVEID         string    `json:"cveId"`
	TargetDigest  string    `json:"targetDigest"`
	Status        string    `json:"status"`
	Justification string    `json:"justification"`
	ApprovedBy    string    `json:"approvedBy"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

type VulnerabilityItem struct {
	CVEID         string                `json:"cveId"`
	PackageName   string                `json:"packageName"`
	InstalledVer  string                `json:"installedVersion"`
	FixedVersion  string                `json:"fixedVersion,omitempty"`
	Severity      VulnerabilitySeverity `json:"severity"`
	CVSSScore     float64               `json:"cvssScore"`
	PublishedDate time.Time             `json:"publishedDate"`
	HasFix        bool                  `json:"hasFix"`
	Exploitation  ExploitationEvidence  `json:"exploitation,omitempty"`
	IsBackdoor    bool                  `json:"isBackdoor,omitempty"`
	Priority      PriorityClass         `json:"priority,omitempty"`
}

type EmbeddedSecretFinding struct {
	RuleID      string `json:"ruleId"`
	Description string `json:"description"`
	FilePath    string `json:"filePath"`
	SecretType  string `json:"secretType"`
}

type SBOMAttestation struct {
	Format       string    `json:"format"`
	Signature    string    `json:"signature"`
	SignerKeyID  string    `json:"signerKeyId"`
	GeneratedAt  time.Time `json:"generatedAt"`
	PackageCount int       `json:"packageCount"`
}

type SecurityExceptionWaiver struct {
	ExceptionID  string    `json:"exceptionId"`
	CVEID        string    `json:"cveId"`
	ApprovedBy   string    `json:"approvedBy"`
	Reason       string    `json:"reason"`
	Compensating string    `json:"compensatingControls"`
	ExpiresAt    time.Time `json:"expiresAt"`
}

type ImageScanCommand struct {
	ImageDigest     string                   `json:"imageDigest"`
	ImageRepository string                   `json:"imageRepository"`
	TenantID        string                   `json:"tenantId"`
	Namespace       string                   `json:"namespace"`
	Vulnerabilities []VulnerabilityItem      `json:"vulnerabilities"`
	SecretsFound    []EmbeddedSecretFinding  `json:"secretsFound,omitempty"`
	SBOM            *SBOMAttestation         `json:"sbom,omitempty"`
	ActiveWaiver    *SecurityExceptionWaiver `json:"activeWaiver,omitempty"`
	PodSlots        []PodSlotImageRef        `json:"podSlots,omitempty"`
	Provenance      *ProvenanceAttestation   `json:"provenance,omitempty"`
	VEXStatements   []VEXStatement           `json:"vexStatements,omitempty"`
	Exposure        WorkloadExposure         `json:"exposure,omitempty"`
}

type ImageAdmissionVerdict struct {
	Admitted         bool          `json:"admitted"`
	ScanStatus       ScanStatus    `json:"scanStatus"`
	BlockReason      string        `json:"blockReason,omitempty"`
	CriticalCVECount int           `json:"criticalCveCount"`
	HighCVECount     int           `json:"highCveCount"`
	SecretsCount     int           `json:"secretsCount"`
	HasValidSBOM     bool          `json:"hasValidSbom"`
	WaiverApplied    bool          `json:"waiverApplied"`
	HighestPriority  PriorityClass `json:"highestPriority,omitempty"`
	SlotViolations   []string      `json:"slotViolations,omitempty"`
	Prio0Count       int           `json:"prio0Count"`
	Prio1Count       int           `json:"prio1Count"`
	Prio2Count       int           `json:"prio2Count"`
	PrioUCount       int           `json:"prioUCount"`
}
