package tests

import (
	"testing"

	"internal-developer-platform/src/features/role-based-access-control/domain/machines"
)

func TestRoleBasedAccessControlMachine_Transitions(t *testing.T) {
	sm := machines.NewTokenLifecycleMachine(machines.TokenStateUnauthenticated)
	if sm.CurrentState() != machines.TokenStateUnauthenticated {
		t.Fatalf("expected Unauthenticated, got %s", sm.CurrentState())
	}

	state, ok := sm.Transition(machines.EventAuthenticate)
	if !ok || state != machines.TokenStateActive {
		t.Fatalf("expected transition to Active, got %s (ok=%v)", state, ok)
	}

	state, ok = sm.Transition(machines.EventRevoke)
	if !ok || state != machines.TokenStateRevoked {
		t.Fatalf("expected transition to Revoked, got %s (ok=%v)", state, ok)
	}

	state, ok = sm.Transition(machines.EventAuthenticate)
	if ok || state != machines.TokenStateRevoked {
		t.Fatalf("expected terminal state rejection, got ok=%v, state=%s", ok, state)
	}
}
