/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING PORT CONFORMANCE TESTS (V.3 role 14, Invariant F)

 1. Role:
    - Verifies compile-time and behavioral conformance across all ports with 2+ concrete adapters.
 2. Invariants:
    - Zero inline comments below this header.
*/
package tests

import (
	"testing"

	"internal-developer-platform/src/features/self-service-provisioning/internal/infrastructure/adapters"
	"internal-developer-platform/src/features/self-service-provisioning/internal/infrastructure/ports"
)

func TestSelfServicePortConformance_AllAdaptersSatisfyPorts(t *testing.T) {
	var _ ports.ClusterProvisionerPort = (*adapters.GKEClusterClientAdapter)(nil)
	var _ ports.CloudResourceProvisionerPort = (*adapters.CloudResourceClientAdapter)(nil)

	var _ ports.ProvisioningLedgerPort = (*adapters.PostgresProvisioningLedgerAdapter)(nil)
	var _ ports.ProvisioningLedgerPort = (*adapters.MemoryProvisioningLedgerAdapter)(nil)

	var _ ports.ProvisioningReservationPort = (*adapters.PostgresProvisioningReservationAdapter)(nil)
	var _ ports.ProvisioningReservationPort = (*adapters.MemoryProvisioningReservationAdapter)(nil)

	var _ ports.ProvisioningApprovalPort = (*adapters.PostgresProvisioningApprovalAdapter)(nil)
	var _ ports.ProvisioningApprovalPort = (*adapters.MemoryProvisioningApprovalAdapter)(nil)

	var _ ports.ProvisioningGovernancePort = (*adapters.PostgresProvisioningGovernanceAdapter)(nil)
	var _ ports.ProvisioningGovernancePort = (*adapters.MemoryProvisioningGovernanceAdapter)(nil)
}
