package tests

import (
	"testing"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/machines"
)

func TestTransitionSyncStateValidFlow(t *testing.T) {
	current := contracts.SyncPhasePending

	step1, err := machines.TransitionSyncState(current, machines.EventStartSync)
	if err != nil || step1 != contracts.SyncPhaseSyncing {
		t.Fatalf("expected transition to SYNCING, got %v, err=%v", step1, err)
	}

	step2, err := machines.TransitionSyncState(step1, machines.EventCompleteSync)
	if err != nil || step2 != contracts.SyncPhaseSynced {
		t.Fatalf("expected transition to SYNCED, got %v, err=%v", step2, err)
	}

	step3, err := machines.TransitionSyncState(step2, machines.EventDetectDrift)
	if err != nil || step3 != contracts.SyncPhaseDriftDetected {
		t.Fatalf("expected transition to DRIFT_DETECTED, got %v, err=%v", step3, err)
	}

	step4, err := machines.TransitionSyncState(step3, machines.EventStartSync)
	if err != nil || step4 != contracts.SyncPhaseSyncing {
		t.Fatalf("expected transition back to SYNCING, got %v, err=%v", step4, err)
	}
}

func TestTransitionSyncStateInvalidFlow(t *testing.T) {
	current := contracts.SyncPhasePending

	_, err := machines.TransitionSyncState(current, machines.EventCompleteSync)
	if err == nil {
		t.Fatalf("expected error on illegal transition from PENDING via COMPLETE_SYNC")
	}

	synced := contracts.SyncPhaseSynced
	_, err = machines.TransitionSyncState(synced, machines.EventCompleteSync)
	if err == nil {
		t.Fatalf("expected error on illegal transition from SYNCED via COMPLETE_SYNC")
	}
}

