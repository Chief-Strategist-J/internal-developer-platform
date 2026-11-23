package contracts

import (
	"strings"
	"time"

	"internal-developer-platform/src/shared/errors"
)

type PolicySource string

const (
	PolicySourceGit    PolicySource = "GIT"
	PolicySourceAPI    PolicySource = "API"
	PolicySourceSystem PolicySource = "SYSTEM"
)

type EnforcementPoint string

const (
	EnforcementPointCEL EnforcementPoint = "CEL"
	EnforcementPointPSA EnforcementPoint = "PSA"
	EnforcementPointPC  EnforcementPoint = "PC"
	EnforcementPointBA  EnforcementPoint = "BA"
	EnforcementPointORG EnforcementPoint = "ORG"
	EnforcementPointCI  EnforcementPoint = "CI"
	EnforcementPointACD EnforcementPoint = "ACD"
)

type PolicySeverity string

const (
	SeverityCritical PolicySeverity = "CRITICAL"
	SeverityHigh     PolicySeverity = "HIGH"
	SeverityMedium   PolicySeverity = "MEDIUM"
	SeverityAdvisory PolicySeverity = "ADVISORY"
)

type PolicyLifecycleState string

const (
	LifecycleDraft      PolicyLifecycleState = "DRAFT"
	LifecycleCIOnly     PolicyLifecycleState = "CI_ONLY"
	LifecycleAudit      PolicyLifecycleState = "AUDIT"
	LifecycleWarn       PolicyLifecycleState = "WARN"
	LifecycleDeny       PolicyLifecycleState = "DENY"
	LifecycleRolledBack PolicyLifecycleState = "ROLLED_BACK"
	LifecycleRetired    PolicyLifecycleState = "RETIRED"
)

type ExceptionStatus string

const (
	ExceptionStatusPending ExceptionStatus = "PENDING"
	ExceptionStatusActive  ExceptionStatus = "ACTIVE"
	ExceptionStatusRevoked ExceptionStatus = "REVOKED"
	ExceptionStatusExpired ExceptionStatus = "EXPIRED"
)

const (
	MaxPolicyNameLength    = 256
	MaxExternalIDLength    = 128
	MaxWorkloadIDLength    = 256
	MaxNamespaceLength     = 63
	MinJustificationLength = 10
	DefaultPageSize        = 50
	MaxPageSize            = 100
)

type PolicyRecord struct {
	ID               string
	PublicID         string
	TenantID         string
	Source           PolicySource
	ExternalID       string
	CorrelationID    string
	Name             string
	Description      string
	EnforcementPoint EnforcementPoint
	Severity         PolicySeverity
	LifecycleState   PolicyLifecycleState
	RuleExpression   string
	Exceptable       bool
	DetectionPair    string
	Version          int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

func (p PolicyRecord) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return errors.NewValidationError("policy id must not be empty")
	}
	if strings.TrimSpace(p.PublicID) == "" {
		return errors.NewValidationError("public id must not be empty")
	}
	if strings.TrimSpace(p.TenantID) == "" {
		return errors.NewValidationError("tenant id must not be empty")
	}
	if strings.TrimSpace(p.CorrelationID) == "" {
		return errors.NewValidationError("correlation id must not be empty")
	}
	trimmedName := strings.TrimSpace(p.Name)
	if trimmedName == "" || len(trimmedName) > MaxPolicyNameLength {
		return errors.NewValidationError("policy name must be between 1 and 256 characters")
	}
	trimmedExtID := strings.TrimSpace(p.ExternalID)
	if trimmedExtID == "" || len(trimmedExtID) > MaxExternalIDLength {
		return errors.NewValidationError("external id must be between 1 and 128 characters")
	}
	if strings.TrimSpace(p.RuleExpression) == "" {
		return errors.NewValidationError("rule expression must not be empty")
	}
	if strings.TrimSpace(p.DetectionPair) == "" {
		return errors.NewValidationError("detection pair must not be empty")
	}
	switch p.Source {
	case PolicySourceGit, PolicySourceAPI, PolicySourceSystem:
	default:
		return errors.NewValidationError("invalid policy source")
	}
	switch p.EnforcementPoint {
	case EnforcementPointCEL, EnforcementPointPSA, EnforcementPointPC, EnforcementPointBA, EnforcementPointORG, EnforcementPointCI, EnforcementPointACD:
	default:
		return errors.NewValidationError("invalid enforcement point")
	}
	switch p.Severity {
	case SeverityCritical, SeverityHigh, SeverityMedium, SeverityAdvisory:
	default:
		return errors.NewValidationError("invalid policy severity")
	}
	switch p.LifecycleState {
	case LifecycleDraft, LifecycleCIOnly, LifecycleAudit, LifecycleWarn, LifecycleDeny, LifecycleRolledBack, LifecycleRetired:
	default:
		return errors.NewValidationError("invalid lifecycle state")
	}
	return nil
}

type PolicyExceptionRecord struct {
	ID            string
	PublicID      string
	TenantID      string
	PolicyID      string
	WorkloadID    string
	Namespace     string
	ApproverID    string
	Justification string
	Status        ExceptionStatus
	ExpiresAt     time.Time
	CorrelationID string
	Version       int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time
}

func (e PolicyExceptionRecord) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return errors.NewValidationError("exception id must not be empty")
	}
	if strings.TrimSpace(e.PublicID) == "" {
		return errors.NewValidationError("public id must not be empty")
	}
	if strings.TrimSpace(e.TenantID) == "" {
		return errors.NewValidationError("tenant id must not be empty")
	}
	if strings.TrimSpace(e.PolicyID) == "" {
		return errors.NewValidationError("policy id must not be empty")
	}
	trimmedWorkload := strings.TrimSpace(e.WorkloadID)
	if trimmedWorkload == "" || len(trimmedWorkload) > MaxWorkloadIDLength {
		return errors.NewValidationError("workload id must be between 1 and 256 characters")
	}
	trimmedNamespace := strings.TrimSpace(e.Namespace)
	if trimmedNamespace == "" || len(trimmedNamespace) > MaxNamespaceLength {
		return errors.NewValidationError("namespace must be between 1 and 63 characters")
	}
	if strings.TrimSpace(e.ApproverID) == "" {
		return errors.NewValidationError("approver id must not be empty")
	}
	if len(strings.TrimSpace(e.Justification)) < MinJustificationLength {
		return errors.NewValidationError("justification must be at least 10 characters")
	}
	if !e.ExpiresAt.After(e.CreatedAt) {
		return errors.NewValidationError("expires_at must be strictly after created_at")
	}
	switch e.Status {
	case ExceptionStatusPending, ExceptionStatusActive, ExceptionStatusRevoked, ExceptionStatusExpired:
	default:
		return errors.NewValidationError("invalid exception status")
	}
	return nil
}

type KeysetFilterCriteria struct {
	TenantID         string
	LifecycleState   PolicyLifecycleState
	EnforcementPoint EnforcementPoint
	CursorID         string
	PageSize         int
}
