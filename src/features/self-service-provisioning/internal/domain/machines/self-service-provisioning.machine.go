/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING STATE MACHINE (V.3 role 08)

 1. Role:
    - Enforces valid lifecycle transitions for provisioning sagas, EC-07 ORPHANED_DIRTY, and EC-13 serialization.
 2. Invariants:
    - Pure stateless transition table; zero IO, zero inline comments below this header.
*/
package machines

import (
	"fmt"

	"internal-developer-platform/src/shared/errors"
)

type ProvisioningState string

const (
	StatePending         ProvisioningState = "PENDING"
	StateProvisioning    ProvisioningState = "PROVISIONING"
	StateReady           ProvisioningState = "READY"
	StateFailed          ProvisioningState = "FAILED"
	StateCompensating    ProvisioningState = "COMPENSATING"
	StateOrphanedDirty   ProvisioningState = "ORPHANED_DIRTY"
	StateDecommissioning ProvisioningState = "DECOMMISSIONING"
	StateDecommissioned  ProvisioningState = "DECOMMISSIONED"
)

func ValidateTransition(from ProvisioningState, to ProvisioningState) error {
	switch from {
	case StatePending:
		if to == StateProvisioning || to == StateFailed {
			return nil
		}
	case StateProvisioning:
		if to == StateReady || to == StateFailed || to == StateCompensating {
			return nil
		}
	case StateReady:
		if to == StateDecommissioning {
			return nil
		}
	case StateCompensating:
		if to == StateFailed || to == StateOrphanedDirty {
			return nil
		}
	case StateDecommissioning:
		if to == StateDecommissioned || to == StateOrphanedDirty {
			return nil
		}
	}
	return errors.NewConflictError(fmt.Sprintf("invalid provisioning state transition: %s -> %s", from, to))
}
