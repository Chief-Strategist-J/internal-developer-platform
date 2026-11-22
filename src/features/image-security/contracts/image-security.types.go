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
	ScanStatusPending    ScanStatus = "PENDING"
	ScanStatusScanning   ScanStatus = "SCANNING"
	ScanStatusPassed     ScanStatus = "PASSED"
	ScanStatusBlocked    ScanStatus = "BLOCKED"
	ScanStatusException  ScanStatus = "EXCEPTION_ACTIVE"
)

type VulnerabilityItem struct {
	CVEID         string                `json:"cveId"`
	PackageName   string                `json:"packageName"`
	InstalledVer  string                `json:"installedVersion"`
	FixedVersion  string                `json:"fixedVersion,omitempty"`
	Severity      VulnerabilitySeverity `json:"severity"`
	CVSSScore     float64               `json:"cvssScore"`
	PublishedDate time.Time             `json:"publishedDate"`
	HasFix        bool                  `json:"hasFix"`
}

type EmbeddedSecretFinding struct {
	RuleID      string `json:"ruleId"`
	Description string `json:"description"`
	FilePath    string `json:"filePath"`
	SecretType  string `json:"secretType"`
}

type SBOMAttestation struct {
	Format      string    `json:"format"`
	Signature   string    `json:"signature"`
	SignerKeyID string    `json:"signerKeyId"`
	GeneratedAt time.Time `json:"generatedAt"`
	PackageCount int      `json:"packageCount"`
}

type SecurityExceptionWaiver struct {
	ExceptionID    string    `json:"exceptionId"`
	CVEID          string    `json:"cveId"`
	ApprovedBy     string    `json:"approvedBy"`
	Reason         string    `json:"reason"`
	Compensating   string    `json:"compensatingControls"`
	ExpiresAt      time.Time `json:"expiresAt"`
}

type ImageScanCommand struct {
	ImageDigest     string                    `json:"imageDigest"`
	ImageRepository string                    `json:"imageRepository"`
	TenantID        string                    `json:"tenantId"`
	Namespace       string                    `json:"namespace"`
	Vulnerabilities []VulnerabilityItem       `json:"vulnerabilities"`
	SecretsFound    []EmbeddedSecretFinding   `json:"secretsFound,omitempty"`
	SBOM            *SBOMAttestation          `json:"sbom,omitempty"`
	ActiveWaiver    *SecurityExceptionWaiver  `json:"activeWaiver,omitempty"`
}

type ImageAdmissionVerdict struct {
	Admitted         bool       `json:"admitted"`
	ScanStatus       ScanStatus `json:"scanStatus"`
	BlockReason      string     `json:"blockReason,omitempty"`
	CriticalCVECount int        `json:"criticalCveCount"`
	HighCVECount     int        `json:"highCveCount"`
	SecretsCount     int        `json:"secretsCount"`
	HasValidSBOM     bool       `json:"hasValidSbom"`
	WaiverApplied    bool       `json:"waiverApplied"`
}
