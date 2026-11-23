/*
ALGORITHM & ARCHITECTURE BLUEPRINT: POLICY LIFECYCLE FINITE STATE MACHINE

1. Architecture Role:
   - Step 03 Domain Machine: Enforces the Policy-as-Code lifecycle state transitions (§5 in business-logic.md).
   - Validates staged ring promotion: DRAFT ──> CI_ONLY ──> AUDIT ──> WARN ──> DENY.
   - Enforces the invariant guard: Transition from WARN to DENY requires active violation count == 0.
   - Any active or staged policy may transition to ROLLED_BACK.
   - Adheres strictly to 06-writing-conditions.md (guard clauses first, parenthesized compound conditions).
   - Zero inline comments inside method bodies; pure deterministic domain logic.
*/
package machines

import (
	"fmt"

	"internal-developer-platform/src/features/policy-as-code/contracts"
	"internal-developer-platform/src/shared/errors"
)

type PolicyLifecycleEvent string

const (
	EventPromoteToCIOnly PolicyLifecycleEvent = "PROMOTE_TO_CI_ONLY"
	EventPromoteToAudit  PolicyLifecycleEvent = "PROMOTE_TO_AUDIT"
	EventPromoteToWarn   PolicyLifecycleEvent = "PROMOTE_TO_WARN"
	EventPromoteToDeny   PolicyLifecycleEvent = "PROMOTE_TO_DENY"
	EventRollback        PolicyLifecycleEvent = "ROLLBACK"
	EventRetire          PolicyLifecycleEvent = "RETIRE"
	EventReopenDraft     PolicyLifecycleEvent = "REOPEN_DRAFT"
)

func TransitionPolicyLifecycle(
	currentState contracts.PolicyLifecycleState,
	event PolicyLifecycleEvent,
	activeViolations int,
) (contracts.PolicyLifecycleState, error) {
	if event == EventRollback {
		if currentState == contracts.LifecycleRetired {
			return currentState, errors.NewConflictError("retired policies cannot be rolled back")
		}
		if currentState == contracts.LifecycleRolledBack {
			return currentState, errors.NewConflictError("policy is already rolled back")
		}
		return contracts.LifecycleRolledBack, nil
	}

	if event == EventRetire {
		if (currentState == contracts.LifecycleDeny) || (currentState == contracts.LifecycleRolledBack) {
			return contracts.LifecycleRetired, nil
		}
		return currentState, errors.NewConflictError(
			fmt.Sprintf("cannot retire policy in state %s; retirement requires DENY or ROLLED_BACK", currentState),
		)
	}

	switch currentState {
	case contracts.LifecycleDraft:
		if event == EventPromoteToCIOnly {
			return contracts.LifecycleCIOnly, nil
		}
	case contracts.LifecycleCIOnly:
		if event == EventPromoteToAudit {
			return contracts.LifecycleAudit, nil
		}
	case contracts.LifecycleAudit:
		if event == EventPromoteToWarn {
			return contracts.LifecycleWarn, nil
		}
	case contracts.LifecycleWarn:
		if event == EventPromoteToDeny {
			if activeViolations > 0 {
				return currentState, errors.NewConflictError(
					fmt.Sprintf("cannot promote policy to DENY: %d active violations must reach 0 first", activeViolations),
				)
			}
			return contracts.LifecycleDeny, nil
		}
	case contracts.LifecycleRolledBack:
		if event == EventReopenDraft {
			return contracts.LifecycleDraft, nil
		}
	case contracts.LifecycleDeny, contracts.LifecycleRetired:
		return currentState, errors.NewConflictError(
			fmt.Sprintf("no promotion events permitted from state %s", currentState),
		)
	}

	return currentState, errors.NewValidationError(
		fmt.Sprintf("illegal policy lifecycle transition from %s via %s", currentState, event),
	)
}
