/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DEPLOYMENT <-> DISASTER RECOVERY BRIDGE (X-03, GO-INT-001)

 1. Role:
    - Implements deploymentstrategies.DRGuardBridge using disasterrecovery.DRBridgeProvider.
 2. Invariants:
    - Root index imports only; zero internal/ imports.
    - Zero inline comments below header.
*/
package bridges

import (
	"context"

	deploymentstrategies "internal-developer-platform/src/features/deployment-strategies"
	disasterrecovery "internal-developer-platform/src/features/disaster-recovery"
)

type DeploymentDRBridge struct {
	dr disasterrecovery.DRBridgeProvider
}

func NewDeploymentDRBridge(dr disasterrecovery.DRBridgeProvider) *DeploymentDRBridge {
	return &DeploymentDRBridge{dr: dr}
}

func (b *DeploymentDRBridge) IsWorkloadFenced(ctx context.Context, tenantID, workloadID string) (bool, error) {
	if b.dr == nil {
		return false, nil
	}
	return b.dr.IsWorkloadFenced(ctx, tenantID, workloadID)
}

var _ deploymentstrategies.DRGuardBridge = (*DeploymentDRBridge)(nil)
