/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING ADAPTER (V.3 role 11, Invariant A)

 1. Role:
    - Concrete adapter implementation separated per V.4 Invariant A.
 2. Invariants:
    - Zero inline comments below this header.
*/
package adapters

import (
	"context"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/infra/clients/cloudresource"
	"internal-developer-platform/src/infra/observability"
)

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
