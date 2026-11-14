package tests

import (
	"testing"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/domain/machines"
)

func TestTransitionWorkloadStateFlow(t *testing.T) {
	current := contracts.StatusSubmitted

	s1, err := machines.TransitionWorkloadState(current, machines.EventValidate)
	if err != nil || s1 != contracts.StatusValidated {
		t.Fatalf("expected VALIDATED, got %v, err=%v", s1, err)
	}

	s2, err := machines.TransitionWorkloadState(s1, machines.EventAdmit)
	if err != nil || s2 != contracts.StatusAdmitted {
		t.Fatalf("expected ADMITTED, got %v, err=%v", s2, err)
	}

	s3, err := machines.TransitionWorkloadState(s2, machines.EventActivate)
	if err != nil || s3 != contracts.StatusActive {
		t.Fatalf("expected ACTIVE, got %v, err=%v", s3, err)
	}

	s4, err := machines.TransitionWorkloadState(s3, machines.EventReject)
	if err != nil || s4 != contracts.StatusRejected {
		t.Fatalf("expected REJECTED, got %v, err=%v", s4, err)
	}
}

func TestTransitionWorkloadStateInvalid(t *testing.T) {
	current := contracts.StatusSubmitted
	_, err := machines.TransitionWorkloadState(current, machines.EventActivate)
	if err == nil {
		t.Errorf("expected error activating unvalidated workload")
	}
}

