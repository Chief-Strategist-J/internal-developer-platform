/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING CONTRACTS (V.3 role 04)

 1. Role:
    - Defines internal domain DTOs and request models for cluster and cloud resource offerings.
 2. Invariants:
    - Pure typed structs without IO or comments inside bodies.
*/
package contracts

import (
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
