package machines

import (
	"fmt"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/shared/errors"
)

type TransitionEvent string

const (
	EventStartSync        TransitionEvent = "START_SYNC"
	EventCompleteSync     TransitionEvent = "COMPLETE_SYNC"
	EventFailSync         TransitionEvent = "FAIL_SYNC"
	EventDetectDrift      TransitionEvent = "DETECT_DRIFT"
	EventMarkDegraded     TransitionEvent = "MARK_DEGRADED"
	EventTriggerRollback  TransitionEvent = "TRIGGER_ROLLBACK"
	EventCompleteRollback TransitionEvent = "COMPLETE_ROLLBACK"
	EventFailRollback     TransitionEvent = "FAIL_ROLLBACK"
)

func TransitionSyncState(current contracts.SyncPhase, event TransitionEvent) (contracts.SyncPhase, error) {
	switch current {
	case contracts.SyncPhasePending:
		switch event {
		case EventStartSync:
			return contracts.SyncPhaseSyncing, nil
		case EventFailSync:
			return contracts.SyncPhaseFailed, nil
		}
	case contracts.SyncPhaseSyncing:
		switch event {
		case EventCompleteSync:
			return contracts.SyncPhaseSynced, nil
		case EventFailSync:
			return contracts.SyncPhaseFailed, nil
		case EventTriggerRollback:
			return contracts.SyncPhaseRollingBack, nil
		}
	case contracts.SyncPhaseSynced:
		switch event {
		case EventStartSync:
			return contracts.SyncPhaseSyncing, nil
		case EventDetectDrift:
			return contracts.SyncPhaseDriftDetected, nil
		case EventTriggerRollback:
			return contracts.SyncPhaseRollingBack, nil
		}
	case contracts.SyncPhaseDriftDetected:
		switch event {
		case EventStartSync:
			return contracts.SyncPhaseSyncing, nil
		case EventFailSync:
			return contracts.SyncPhaseFailed, nil
		case EventTriggerRollback:
			return contracts.SyncPhaseRollingBack, nil
		}
	case contracts.SyncPhaseRollingBack:
		switch event {
		case EventCompleteRollback:
			return contracts.SyncPhaseSynced, nil
		case EventFailRollback:
			return contracts.SyncPhaseFailed, nil
		}
	case contracts.SyncPhaseFailed:
		switch event {
		case EventStartSync:
			return contracts.SyncPhaseSyncing, nil
		case EventTriggerRollback:
			return contracts.SyncPhaseRollingBack, nil
		}
	}
	return current, errors.NewConflictError(fmt.Sprintf("illegal transition from %s via %s", current, event))
}
