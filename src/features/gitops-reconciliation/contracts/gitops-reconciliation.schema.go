package contracts

import (
	"regexp"

	"internal-developer-platform/src/shared/errors"
)

var (
	rxUUID       = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]{1,128}$`)
	rxRevision   = regexp.MustCompile(`^[a-zA-Z0-9_\-\.\/]{1,128}$`)
	rxAppName    = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?$`)
)

func ValidateSyncCommand(cmd SyncCommand) error {
	if cmd.ID == "" || !rxUUID.MatchString(cmd.ID) {
		return errors.NewValidationError("id must match alphanumeric identifier format")
	}
	if cmd.TenantID == "" || !rxUUID.MatchString(cmd.TenantID) {
		return errors.NewValidationError("tenantId must match alphanumeric identifier format")
	}
	if cmd.ServiceID == "" || !rxUUID.MatchString(cmd.ServiceID) {
		return errors.NewValidationError("serviceId must match alphanumeric identifier format")
	}
	if cmd.AppName == "" || !rxAppName.MatchString(cmd.AppName) {
		return errors.NewValidationError("appName must be RFC-1123 compliant")
	}
	if cmd.TargetRevision == "" || !rxRevision.MatchString(cmd.TargetRevision) {
		return errors.NewValidationError("targetRevision must match valid git revision format")
	}
	if cmd.IdempotencyKey == "" || len(cmd.IdempotencyKey) > 256 {
		return errors.NewValidationError("idempotencyKey must be between 1 and 256 characters")
	}
	return nil
}

