package tests

import (
	"testing"

	catalogContracts "internal-developer-platform/src/features/service-catalog/contracts"
	catalogRules "internal-developer-platform/src/features/service-catalog/domain/rules"
)

func TestCanTransitionServiceStatus(t *testing.T) {
	if !catalogRules.CanTransitionServiceStatus(catalogContracts.ServiceStatusDraft, catalogContracts.ServiceStatusActive) {
		t.Errorf("expected transition draft -> active to be valid")
	}
	if catalogRules.CanTransitionServiceStatus(catalogContracts.ServiceStatusDraft, catalogContracts.ServiceStatusDeprecated) {
		t.Errorf("expected transition draft -> deprecated to be invalid")
	}
}
