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

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/infra/clients/cloudresource"
	"internal-developer-platform/src/infra/clients/cluster"
	"internal-developer-platform/src/infra/observability"
)

type GKEClusterClientAdapter struct {
	client *cluster.GKEClusterClient
}

func NewGKEClusterClientAdapter(client *cluster.GKEClusterClient) *GKEClusterClientAdapter {
	return &GKEClusterClientAdapter{client: client}
}

func (a *GKEClusterClientAdapter) ProvisionCluster(ctx context.Context, spec contracts.ClusterSpec) (*contracts.ClusterRecord, error) {
	return observability.RunTracedClient(ctx, "self_service.client.gke.provision_cluster", func(ctx context.Context) (*contracts.ClusterRecord, error) {
		rec, err := a.client.ProvisionCluster(ctx, spec)
		if err != nil {
			observability.GetLogger().Error(ctx, "gke client provision cluster failed",
				observability.String("cluster_id", spec.ClusterID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}
		return rec, nil
	}, attribute.String("cluster.id", spec.ClusterID), attribute.String("project.id", spec.ProjectID))
}

func (a *GKEClusterClientAdapter) GetCluster(ctx context.Context, clusterID string) (*contracts.ClusterRecord, error) {
	return observability.RunTracedClient(ctx, "self_service.client.gke.get_cluster", func(ctx context.Context) (*contracts.ClusterRecord, error) {
		rec, err := a.client.GetCluster(ctx, clusterID)
		if err != nil {
			observability.GetLogger().Warn(ctx, "gke client get cluster failed",
				observability.String("cluster_id", clusterID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}
		return rec, nil
	}, attribute.String("cluster.id", clusterID))
}

func (a *GKEClusterClientAdapter) ValidatePosture(ctx context.Context, clusterID string) (*contracts.ClusterPostureReport, error) {
	return observability.RunTracedClient(ctx, "self_service.client.gke.validate_posture", func(ctx context.Context) (*contracts.ClusterPostureReport, error) {
		report, err := a.client.ValidatePosture(ctx, clusterID)
		if err != nil {
			observability.GetLogger().Warn(ctx, "gke client validate posture failed",
				observability.String("cluster_id", clusterID),
				observability.String("error", err.Error()),
			)
			return nil, err
		}
		return report, nil
	}, attribute.String("cluster.id", clusterID))
}

type CloudResourceClientAdapter struct {
	client *cloudresource.CloudResourceClient
}

func NewCloudResourceClientAdapter(client *cloudresource.CloudResourceClient) *CloudResourceClientAdapter {
	return &CloudResourceClientAdapter{client: client}
}

func (a *CloudResourceClientAdapter) ProvisionWorkloadIdentity(ctx context.Context, spec contracts.WorkloadIdentitySpec) (*contracts.ResourceManifestResult, error) {
	return observability.RunTracedClient(ctx, "self_service.client.cnrm.provision_workload_identity", func(ctx context.Context) (*contracts.ResourceManifestResult, error) {
		res, err := a.client.ProvisionWorkloadIdentity(ctx, spec)
		if err != nil {
			observability.GetLogger().Error(ctx, "cnrm client provision workload identity failed",
				observability.String("project_id", spec.ProjectID),
				observability.String("ksa_name", spec.KSAName),
				observability.String("error", err.Error()),
			)
			return nil, err
		}
		return res, nil
	}, attribute.String("project.id", spec.ProjectID), attribute.String("ksa.name", spec.KSAName))
}

func (a *CloudResourceClientAdapter) ReconcileLedger(ctx context.Context, activeWorkloadIDs []string) (*contracts.LedgerReconciliationReport, error) {
	return observability.RunTracedClient(ctx, "self_service.client.cnrm.reconcile_ledger", func(ctx context.Context) (*contracts.LedgerReconciliationReport, error) {
		report, err := a.client.ReconcileLedger(ctx, activeWorkloadIDs)
		if err != nil {
			observability.GetLogger().Error(ctx, "cnrm client reconcile ledger failed",
				observability.String("error", err.Error()),
			)
			return nil, err
		}
		return report, nil
	}, attribute.Int("active_workloads.count", len(activeWorkloadIDs)))
}
