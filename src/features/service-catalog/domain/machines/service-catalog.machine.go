package machines

import (
	catalogContracts "internal-developer-platform/src/features/service-catalog/contracts"
	catalogRules "internal-developer-platform/src/features/service-catalog/domain/rules"
	"internal-developer-platform/src/shared/errors"
)

type ServiceStateMachine struct{}

func NewServiceStateMachine() *ServiceStateMachine {
	return &ServiceStateMachine{}
}

func (m *ServiceStateMachine) TransitionTo(current catalogContracts.ServiceStatus, next catalogContracts.ServiceStatus) (catalogContracts.ServiceStatus, error) {
	if !catalogRules.CanTransitionServiceStatus(current, next) {
		return current, errors.NewConflictError("invalid service status transition")
	}
	return next, nil
}
