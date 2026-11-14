package adapters

import (
	"context"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/rules"
)

type KubernetesClientAdapter struct {
	clusterEndpoint string
}

func NewKubernetesClientAdapter(endpoint string) *KubernetesClientAdapter {
	return &KubernetesClientAdapter{clusterEndpoint: endpoint}
}

func (k *KubernetesClientAdapter) ApplyDeployment(ctx context.Context, cmd contracts.WorkloadProvisionCommand, mutations rules.WorkloadMutations) error {
	return nil
}

func (k *KubernetesClientAdapter) ApplyPDB(ctx context.Context, namespace string, workloadName string, minAvailable int) error {
	return nil
}

func (k *KubernetesClientAdapter) GetWorkloadHealth(ctx context.Context, namespace string, workloadName string) (contracts.WorkloadStatus, error) {
	return contracts.StatusActive, nil
}

