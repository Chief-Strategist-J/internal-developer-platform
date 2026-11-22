package contracts

import (
	"strings"

	"internal-developer-platform/src/shared/errors"
)

func ValidateImageScanCommand(cmd ImageScanCommand) error {
	if strings.TrimSpace(cmd.ImageDigest) == "" {
		return errors.NewValidationError("imageDigest must not be empty")
	}
	if !strings.HasPrefix(cmd.ImageDigest, "sha256:") {
		return errors.NewValidationError("imageDigest must be a valid sha256 reference")
	}
	if strings.TrimSpace(cmd.TenantID) == "" {
		return errors.NewValidationError("tenantId must not be empty")
	}
	if strings.TrimSpace(cmd.Namespace) == "" {
		return errors.NewValidationError("namespace must not be empty")
	}
	return nil
}
