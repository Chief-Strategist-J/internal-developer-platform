package servicecatalog

import (
	"internal-developer-platform/src/shared/errors"
)

func ValidateRegisterServiceInput(input RegisterServiceInput) error {
	if input.ID == "" {
		return errors.NewValidationError("id is required")
	}
	if input.TenantID == "" {
		return errors.NewValidationError("tenantId is required")
	}
	if input.Name == "" {
		return errors.NewValidationError("name is required")
	}
	if input.Owner == "" {
		return errors.NewValidationError("owner is required")
	}
	return nil
}
