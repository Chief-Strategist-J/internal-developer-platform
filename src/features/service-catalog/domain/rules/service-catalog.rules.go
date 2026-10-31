package rules

import (
	catalogContracts "internal-developer-platform/src/features/service-catalog/contracts"
)

func CanTransitionServiceStatus(from catalogContracts.ServiceStatus, to catalogContracts.ServiceStatus) bool {
	switch from {
	case catalogContracts.ServiceStatusDraft:
		return to == catalogContracts.ServiceStatusActive || to == catalogContracts.ServiceStatusRetired
	case catalogContracts.ServiceStatusActive:
		return to == catalogContracts.ServiceStatusDeprecated || to == catalogContracts.ServiceStatusRetired
	case catalogContracts.ServiceStatusDeprecated:
		return to == catalogContracts.ServiceStatusRetired
	default:
		return false
	}
}
