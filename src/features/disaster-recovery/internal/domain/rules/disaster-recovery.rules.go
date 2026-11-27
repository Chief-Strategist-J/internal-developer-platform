// Package rules evaluates pure business rules, invariant validations, and policy gates for DR.
//
// Architectural Blueprint:
//   - Step 07 Rules: Business logic decision tables, invariant verifications (DR-INV-01 to DR-INV-12),
//     and quorum enforcement (D-DECLARE two-person approval).
//   - Zero inline comments inside method bodies.
package rules

import (
	"fmt"

	"internal-developer-platform/src/shared/errors"
)

func ValidateTwoPersonDeclaration(declarer string, secondApprover string) error {
	if declarer == "" {
		return errors.NewValidationError("declarer cannot be empty")
	}
	if secondApprover == "" {
		return errors.NewValidationError("second approver is mandatory for stateful disaster declaration (D-DECLARE)")
	}
	if declarer == secondApprover {
		return errors.NewConflictError("declarer and second approver must be distinct individuals (two-person rule)")
	}
	return nil
}

func ValidateTwoPersonFailover(initiator string, approver string) error {
	if initiator == "" {
		return errors.NewValidationError("initiator cannot be empty")
	}
	if approver == "" {
		return errors.NewValidationError("secondary approval is mandatory to execute failover (D-DECLARE)")
	}
	if initiator == approver {
		return errors.NewConflictError("failover initiator and approver must be distinct individuals")
	}
	return nil
}

func VerifyReplicationLagWithinRPO(lagSeconds int, targetRPOSeconds int) error {
	if lagSeconds > targetRPOSeconds {
		return errors.NewConflictError(
			fmt.Sprintf("measured replication lag (%d s) exceeds target RPO ceiling (%d s) - failover blocked", lagSeconds, targetRPOSeconds),
		)
	}
	return nil
}

func VerifyFencingEpochPrecondition(currentEpoch int64, expectedEpoch int64) error {
	if currentEpoch <= expectedEpoch {
		return errors.NewConflictError(
			fmt.Sprintf("fencing epoch not strictly incremented (current: %d, expected > %d) - split brain protection tripped", currentEpoch, expectedEpoch),
		)
	}
	return nil
}

func ValidateFailbackPrerequisites(divergenceCount int, reverseLagSeconds int) error {
	if divergenceCount > 0 {
		return errors.NewConflictError(
			fmt.Sprintf("failback rejected: unresolved dataset divergence detected (%d divergent records)", divergenceCount),
		)
	}
	if reverseLagSeconds != 0 {
		return errors.NewConflictError(
			fmt.Sprintf("failback rejected: reverse replication lag must be 0 (current: %d s)", reverseLagSeconds),
		)
	}
	return nil
}
