/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING SERVICE (V.3 role 06)

 1. Role:
    - Stateless orchestrator delegating cluster and cloud resource offerings to injected ports.
 2. Invariants:
    - Zero mutable state beyond injected dependencies (F-02); zero goroutines or tickers (G-11).
    - Zero inline comments below this header.
*/
package service

import (
	"context"
	"fmt"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/features/self-service-provisioning/internal/infrastructure/ports"
)

type SelfServiceProvisioningService struct {
	clusterPort       ports.ClusterProvisionerPort
	cloudResourcePort ports.CloudResourceProvisionerPort
}

func NewSelfServiceProvisioningService(clusterPort ports.ClusterProvisionerPort, cloudResourcePort ports.CloudResourceProvisionerPort) *SelfServiceProvisioningService {
	return &SelfServiceProvisioningService{
		clusterPort:       clusterPort,
		cloudResourcePort: cloudResourcePort,
	}
}

func (s *SelfServiceProvisioningService) ProvisionCluster(ctx context.Context, spec contracts.ClusterSpec) (*contracts.ClusterRecord, error) {
	if s.clusterPort == nil {
		return nil, fmt.Errorf("cluster provisioner port is not configured")
	}
	return s.clusterPort.ProvisionCluster(ctx, spec)
}

func (s *SelfServiceProvisioningService) GetCluster(ctx context.Context, clusterID string) (*contracts.ClusterRecord, error) {
	if s.clusterPort == nil {
		return nil, fmt.Errorf("cluster provisioner port is not configured")
	}
	return s.clusterPort.GetCluster(ctx, clusterID)
}

func (s *SelfServiceProvisioningService) ValidateClusterPosture(ctx context.Context, clusterID string) (*contracts.ClusterPostureReport, error) {
	if s.clusterPort == nil {
		return nil, fmt.Errorf("cluster provisioner port is not configured")
	}
	return s.clusterPort.ValidatePosture(ctx, clusterID)
}

func (s *SelfServiceProvisioningService) ProvisionWorkloadIdentity(ctx context.Context, spec contracts.WorkloadIdentitySpec) (*contracts.ResourceManifestResult, error) {
	if s.cloudResourcePort == nil {
		return nil, fmt.Errorf("cloud resource provisioner port is not configured")
	}
	return s.cloudResourcePort.ProvisionWorkloadIdentity(ctx, spec)
}

func (s *SelfServiceProvisioningService) ReconcileLedger(ctx context.Context, activeWorkloadIDs []string) (*contracts.LedgerReconciliationReport, error) {
	if s.cloudResourcePort == nil {
		return nil, fmt.Errorf("cloud resource provisioner port is not configured")
	}
	return s.cloudResourcePort.ReconcileLedger(ctx, activeWorkloadIDs)
}
