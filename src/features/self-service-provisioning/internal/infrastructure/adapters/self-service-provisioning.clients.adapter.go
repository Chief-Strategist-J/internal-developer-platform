/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING ADAPTERS (V.3 role 11)

 1. Role:
    - Wraps infra/clients/cluster and infra/clients/cloudresource SDK clients.
 2. Invariants:
    - Clients injected at construction (G-05); zero inline comments.
*/
package adapters

import (
	"context"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/infra/clients/cloudresource"
	"internal-developer-platform/src/infra/clients/cluster"
)

type GKEClusterClientAdapter struct {
	client *cluster.GKEClusterClient
}

func NewGKEClusterClientAdapter(client *cluster.GKEClusterClient) *GKEClusterClientAdapter {
	return &GKEClusterClientAdapter{client: client}
}

func (a *GKEClusterClientAdapter) ProvisionCluster(ctx context.Context, spec contracts.ClusterSpec) (*contracts.ClusterRecord, error) {
	return a.client.ProvisionCluster(ctx, spec)
}

func (a *GKEClusterClientAdapter) GetCluster(ctx context.Context, clusterID string) (*contracts.ClusterRecord, error) {
	return a.client.GetCluster(ctx, clusterID)
}

func (a *GKEClusterClientAdapter) ValidatePosture(ctx context.Context, clusterID string) (*contracts.ClusterPostureReport, error) {
	return a.client.ValidatePosture(ctx, clusterID)
}

type CloudResourceClientAdapter struct {
	client *cloudresource.CloudResourceClient
}

func NewCloudResourceClientAdapter(client *cloudresource.CloudResourceClient) *CloudResourceClientAdapter {
	return &CloudResourceClientAdapter{client: client}
}

func (a *CloudResourceClientAdapter) ProvisionWorkloadIdentity(ctx context.Context, spec contracts.WorkloadIdentitySpec) (*contracts.ResourceManifestResult, error) {
	return a.client.ProvisionWorkloadIdentity(ctx, spec)
}

func (a *CloudResourceClientAdapter) ReconcileLedger(ctx context.Context, activeWorkloadIDs []string) (*contracts.LedgerReconciliationReport, error) {
	return a.client.ReconcileLedger(ctx, activeWorkloadIDs)
}
