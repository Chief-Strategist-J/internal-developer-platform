/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PLATFORM REGIME TYPES

1. Entity Contract:
  - Implements data contract v1.2 for platform regime governance (B-1 to B-4).
  - Enforces 5-identifier roles: id (surrogate), public_id (external), tenant_id (scope),
    source + external_id (natural key), correlation_id (causation trace).
  - Strict immutable parameter types and zero comments inside function bodies.
*/
package contracts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type RegimePolicyStatus string

const (
	RegimeStatusDraft      RegimePolicyStatus = "DRAFT"
	RegimeStatusActive     RegimePolicyStatus = "ACTIVE"
	RegimeStatusSuperseded RegimePolicyStatus = "SUPERSEDED"
)

type KMSCustodyMode string

const (
	KMSCustodyCMEK   KMSCustodyMode = "CMEK"
	KMSCustodyEKM    KMSCustodyMode = "EKM"
	KMSCustodyDODKMS KMSCustodyMode = "DOD_KMS"
)

type ClusterMode string

const (
	ClusterModeGKEStandard  ClusterMode = "GKE_STANDARD"
	ClusterModeGKEAutopilot ClusterMode = "GKE_AUTOPILOT"
)

type TenancyMode string

const (
	TenancyModeInternalOnly   TenancyMode = "INTERNAL_ONLY"
	TenancyModeSharedExternal TenancyMode = "SHARED_EXTERNAL"
)

type PlatformRegimePolicyRecord struct {
	ID                   string             `json:"id"`
	PublicID             string             `json:"publicId"`
	TenantID             string             `json:"tenantId"`
	Source               string             `json:"source"`
	ExternalID           string             `json:"externalId"`
	CorrelationID        string             `json:"correlationId"`
	Jurisdiction         string             `json:"jurisdiction"`
	AuditRetentionDays   int                `json:"auditRetentionDays"`
	DataResidencyRegions []string           `json:"dataResidencyRegions"`
	KMSKeyCustodyMode    KMSCustodyMode     `json:"kmsKeyCustodyMode"`
	RTOMinutesTier0      int                `json:"rtoMinutesTier0"`
	RPOMinutesTier0      int                `json:"rpoMinutesTier0"`
	RTOMinutesTier1      int                `json:"rtoMinutesTier1"`
	RPOMinutesTier1      int                `json:"rpoMinutesTier1"`
	ClusterMode          ClusterMode        `json:"clusterMode"`
	PrimaryRegionCount   int                `json:"primaryRegionCount"`
	StandbyRegionCount   int                `json:"standbyRegionCount"`
	TenantIsolationMode  TenancyMode        `json:"tenantIsolationMode"`
	Status               RegimePolicyStatus `json:"status"`
	Version              int64              `json:"version"`
	CreatedAt            time.Time          `json:"createdAt"`
	UpdatedAt            time.Time          `json:"updatedAt"`
}

type CreateRegimePolicyCommand struct {
	TenantID             string         `json:"tenantId"`
	Source               string         `json:"source"`
	ExternalID           string         `json:"externalId"`
	CorrelationID        string         `json:"correlationId"`
	Jurisdiction         string         `json:"jurisdiction"`
	AuditRetentionDays   int            `json:"auditRetentionDays"`
	DataResidencyRegions []string       `json:"dataResidencyRegions"`
	KMSKeyCustodyMode    KMSCustodyMode `json:"kmsKeyCustodyMode"`
	RTOMinutesTier0      int            `json:"rtoMinutesTier0"`
	RPOMinutesTier0      int            `json:"rpoMinutesTier0"`
	RTOMinutesTier1      int            `json:"rtoMinutesTier1"`
	RPOMinutesTier1      int            `json:"rpoMinutesTier1"`
	ClusterMode          ClusterMode    `json:"clusterMode"`
	PrimaryRegionCount   int            `json:"primaryRegionCount"`
	StandbyRegionCount   int            `json:"standbyRegionCount"`
	TenantIsolationMode  TenancyMode    `json:"tenantIsolationMode"`
}

func ComputeRegimePolicyPublicID(rawID string) string {
	hasher := sha256.New()
	hasher.Write([]byte(rawID))
	digest := hex.EncodeToString(hasher.Sum(nil))
	clean := strings.ToUpper(digest[:26])
	return fmt.Sprintf("rgm_%s", clean)
}
