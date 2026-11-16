package machines

import (
	"fmt"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/shared/errors"
)

type WorkloadEvent string

const (
	EventValidate      WorkloadEvent = "VALIDATE"
	EventAdmit         WorkloadEvent = "ADMIT"
	EventApply         WorkloadEvent = "APPLY"
	EventActivate      WorkloadEvent = "ACTIVATE"
	EventReplicasReady WorkloadEvent = "REPLICAS_READY"
	EventDegrade       WorkloadEvent = "DEGRADE"
	EventReject        WorkloadEvent = "REJECT"
)

func TransitionWorkloadState(current contracts.WorkloadStatus, event WorkloadEvent) (contracts.WorkloadStatus, error) {
	switch current {
	case contracts.StatusSubmitted:
		switch event {
		case EventValidate:
			return contracts.StatusValidated, nil
		case EventReject:
			return contracts.StatusRejected, nil
		}
	case contracts.StatusValidated:
		switch event {
		case EventAdmit:
			return contracts.StatusAdmitted, nil
		case EventReject:
			return contracts.StatusRejected, nil
		}
	case contracts.StatusAdmitted:
		switch event {
		case EventApply:
			return contracts.StatusProvisioning, nil
		case EventActivate:
			return contracts.StatusActive, nil
		case EventReject:
			return contracts.StatusRejected, nil
		}
	case contracts.StatusProvisioning:
		switch event {
		case EventReplicasReady:
			return contracts.StatusObservedReady, nil
		case EventActivate:
			return contracts.StatusActive, nil
		case EventDegrade:
			return contracts.StatusDegraded, nil
		case EventReject:
			return contracts.StatusRejected, nil
		}
	case contracts.StatusActive:
		switch event {
		case EventReplicasReady:
			return contracts.StatusObservedReady, nil
		case EventDegrade:
			return contracts.StatusDegraded, nil
		case EventReject:
			return contracts.StatusRejected, nil
		}
	case contracts.StatusObservedReady:
		switch event {
		case EventDegrade:
			return contracts.StatusDegraded, nil
		case EventReject:
			return contracts.StatusRejected, nil
		}
	case contracts.StatusDegraded:
		switch event {
		case EventReplicasReady:
			return contracts.StatusObservedReady, nil
		case EventReject:
			return contracts.StatusRejected, nil
		}
	}
	return current, errors.NewConflictError(fmt.Sprintf("illegal workload transition from %s via %s", current, event))
}
