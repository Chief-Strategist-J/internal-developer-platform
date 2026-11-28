/*
ALGORITHM & ARCHITECTURE BLUEPRINT: ROLLOUT STATE MACHINE (V.3 role 04, P0-05)

1. Purpose:
   - Governs legal state transitions for progressive rollout executions.
   - Enforces deterministic, idempotent FSM guarantees:
     INITIALIZING -> ROUTING_STEP | ABORTED
     ROUTING_STEP -> ANALYZING | ABORTING
     ANALYZING    -> ROUTING_STEP | PROMOTING | PAUSED | ABORTING
     PAUSED       -> ANALYZING | ABORTING
     PROMOTING    -> COMPLETED | ABORTING
     ABORTING     -> ABORTED
     COMPLETED / ABORTED are terminal.
2. Invariants:
   - Rejects any out-of-order or backward transitions with ErrIllegalTransition.
   - Zero inline comments below this header.
*/

package machines

import (
	"fmt"
	"internal-developer-platform/src/features/deployment-strategies/internal/contracts"
)

type StateMachineError struct {
	FromState contracts.RolloutStatus
	ToState   contracts.RolloutStatus
}

func (e *StateMachineError) Error() string {
	return fmt.Sprintf("illegal rollout state transition from %s to %s", e.FromState, e.ToState)
}

func CanTransition(from contracts.RolloutStatus, to contracts.RolloutStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case contracts.RolloutStatusInitializing:
		return to == contracts.RolloutStatusRoutingStep ||
			to == contracts.RolloutStatusAborted

	case contracts.RolloutStatusRoutingStep:
		return to == contracts.RolloutStatusAnalyzing ||
			to == contracts.RolloutStatusAborting

	case contracts.RolloutStatusAnalyzing:
		return to == contracts.RolloutStatusRoutingStep ||
			to == contracts.RolloutStatusPromoting ||
			to == contracts.RolloutStatusPaused ||
			to == contracts.RolloutStatusAborting

	case contracts.RolloutStatusPaused:
		return to == contracts.RolloutStatusAnalyzing ||
			to == contracts.RolloutStatusAborting

	case contracts.RolloutStatusPromoting:
		return to == contracts.RolloutStatusCompleted ||
			to == contracts.RolloutStatusAborting

	case contracts.RolloutStatusAborting:
		return to == contracts.RolloutStatusAborted

	case contracts.RolloutStatusCompleted, contracts.RolloutStatusAborted:
		return false

	default:
		return false
	}
}

func ValidateTransition(from contracts.RolloutStatus, to contracts.RolloutStatus) error {
	if !CanTransition(from, to) {
		return &StateMachineError{FromState: from, ToState: to}
	}
	return nil
}
