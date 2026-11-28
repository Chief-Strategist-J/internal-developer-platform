/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING CONTRACTS (V.3 role 04)

 1. Role:
    - Defines internal domain DTOs and request models for cluster and cloud resource offerings.
 2. Invariants:
    - Pure typed structs without IO or comments inside bodies.
*/
package contracts

import (
	"time"

	"internal-developer-platform/src/infra/clients/cloudresource"
	"internal-developer-platform/src/infra/clients/cluster"
)

type ReconcileLedgerRequest struct {
	ActiveWorkloadIDs []string `json:"activeWorkloadIds"`
}

type ClusterSpec = cluster.GKEClusterSpec
type ClusterRecord = cluster.GKEClusterRecord
type ClusterPostureReport = cluster.ClusterPostureReport
type WorkloadIdentitySpec = cloudresource.WorkloadIdentityBindingSpec
type ResourceManifestResult = cloudresource.ManifestResult
type LedgerReconciliationReport = cloudresource.LedgerReconciliationReport

type ProvisioningIntent struct {
	ID           string `json:"id"`
	PublicID     string `json:"publicId"`
	TenantID     string `json:"tenantId"`
	ResourceKind string `json:"resourceKind"`
	TargetID     string `json:"targetId"`
	Attempt      int    `json:"attempt"`
	Status       string `json:"status"`
	OutcomeID    string `json:"outcomeId,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

type ReservationRequest struct {
	TenantID         string `json:"tenantId"`
	ResourceKind     string `json:"resourceKind"`
	RawName          string `json:"rawName"`
	NormalizedName   string `json:"normalizedName"`
	QuotaUnits       int    `json:"quotaUnits"`
	IPCIDRBlocks     int    `json:"ipCidrBlocks"`
	ClusterIPUtilPct int    `json:"clusterIpUtilPct"`
	TTLSeconds       int    `json:"ttlSeconds"`
}

type ReservationRecord struct {
	ID             string `json:"id"`
	PublicID       string `json:"publicId"`
	TenantID       string `json:"tenantId"`
	ResourceKind   string `json:"resourceKind"`
	NormalizedName string `json:"normalizedName"`
	QuotaUnits     int    `json:"quotaUnits"`
	IPCIDRBlocks   int    `json:"ipCidrBlocks"`
	Status         string `json:"status"`
}

type TenantCapacityCeiling struct {
	TenantID            string `json:"tenantId"`
	MaxQuotaUnits       int    `json:"maxQuotaUnits"`
	MaxIPCIDRBlocks     int    `json:"maxIpCidrBlocks"`
	MaxClusterIPUtilPct int    `json:"maxClusterIpUtilPct"`
}

type PinnedPlan struct {
	TemplateSHA    string `json:"templateSha"`
	PolicySHA      string `json:"policySha"`
	Sizing         string `json:"sizing"`
	NormalizedName string `json:"normalizedName"`
}

type ApprovalRecord struct {
	ID          string    `json:"id"`
	PublicID    string    `json:"publicId"`
	TenantID    string    `json:"tenantId"`
	RequesterID string    `json:"requesterId"`
	ApproverID  string    `json:"approverId"`
	PlanHash    string    `json:"planHash"`
	TemplateSHA string    `json:"templateSha"`
	PolicySHA   string    `json:"policySha"`
	BudgetUnits int       `json:"budgetUnits"`
	Status      string    `json:"status"`
	ExpiresAt   time.Time `json:"expiresAt"`
}
