// Package machines provides the deterministic state machine for Disaster Recovery lifecycles.
//
// Architectural Blueprint:
//   - Step 08 Machine: Deterministic state machine managing DR plan transitions:
//     NOT_READY ⇄ READY
//     READY → DECLARED (two-person) → FENCING → PROMOTING → SCALING → VALIDATING → ROUTING → FAILED_OVER
//     Any step before PROMOTING → ABORTED. After PROMOTING, abort is forbidden.
//     FAILED_OVER → RECONCILING → FAILBACK_READY → FAILING_BACK → NORMAL
//   - 100% comment-free function bodies.
package machines

import (
	"fmt"

	"internal-developer-platform/src/features/disaster-recovery/internal/contracts"
	"internal-developer-platform/src/shared/errors"
)

type DREvent string

const (
	EventMarkReady         DREvent = "MARK_READY"
	EventMarkDegraded      DREvent = "MARK_DEGRADED"
	EventDeclare           DREvent = "DECLARE"
	EventStartFencing      DREvent = "START_FENCING"
	EventStartPromoting    DREvent = "START_PROMOTING"
	EventStartScaling      DREvent = "START_SCALING"
	EventStartValidating   DREvent = "START_VALIDATING"
	EventStartRouting      DREvent = "START_ROUTING"
	EventCompleteFailover  DREvent = "COMPLETE_FAILOVER"
	EventAbort             DREvent = "ABORT"
	EventStartReconcile    DREvent = "START_RECONCILE"
	EventMarkFailbackReady DREvent = "MARK_FAILBACK_READY"
	EventStartFailback     DREvent = "START_FAILBACK"
	EventCompleteFailback  DREvent = "COMPLETE_FAILBACK"
)

func TransitionDRState(current contracts.DRPlanStatus, event DREvent) (contracts.DRPlanStatus, error) {
	switch current {
	case contracts.StatusNotReady:
		if event == EventMarkReady {
			return contracts.StatusReady, nil
		}
	case contracts.StatusReady:
		switch event {
		case EventMarkDegraded:
			return contracts.StatusDegraded, nil
		case EventDeclare:
			return contracts.StatusDeclared, nil
		case EventStartFencing:
			return contracts.StatusFencing, nil
		}
	case contracts.StatusDegraded:
		switch event {
		case EventMarkReady:
			return contracts.StatusReady, nil
		case EventDeclare:
			return contracts.StatusDeclared, nil
		}
	case contracts.StatusDeclared:
		switch event {
		case EventStartFencing:
			return contracts.StatusFencing, nil
		case EventAbort:
			return contracts.StatusAborted, nil
		}
	case contracts.StatusFencing:
		switch event {
		case EventStartPromoting:
			return contracts.StatusPromoting, nil
		case EventAbort:
			return contracts.StatusAborted, nil
		}
	case contracts.StatusPromoting:
		if event == EventStartScaling {
			return contracts.StatusScaling, nil
		}
	case contracts.StatusScaling:
		if event == EventStartValidating {
			return contracts.StatusValidating, nil
		}
	case contracts.StatusValidating:
		if event == EventStartRouting {
			return contracts.StatusRouting, nil
		}
	case contracts.StatusRouting:
		if event == EventCompleteFailover {
			return contracts.StatusFailedOver, nil
		}
	case contracts.StatusFailedOver:
		if event == EventStartReconcile {
			return contracts.StatusReconciling, nil
		}
	case contracts.StatusReconciling:
		if event == EventMarkFailbackReady {
			return contracts.StatusFailbackReady, nil
		}
	case contracts.StatusFailbackReady:
		if event == EventStartFailback {
			return contracts.StatusFailingBack, nil
		}
	case contracts.StatusFailingBack:
		if event == EventCompleteFailback {
			return contracts.StatusNormal, nil
		}
	case contracts.StatusNormal:
		if event == EventMarkReady {
			return contracts.StatusReady, nil
		}
	case contracts.StatusAborted:
		if event == EventMarkReady {
			return contracts.StatusReady, nil
		}
	}

	return current, errors.NewConflictError(
		fmt.Sprintf("illegal DR state transition from '%s' via event '%s'", current, event),
	)
}

func CanAbort(status contracts.DRPlanStatus) bool {
	return status == contracts.StatusDeclared || status == contracts.StatusFencing
}
