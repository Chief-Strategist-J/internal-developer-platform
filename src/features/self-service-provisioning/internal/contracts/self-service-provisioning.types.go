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
