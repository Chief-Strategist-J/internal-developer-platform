package resilience

import (
	"context"
	"time"

	"internal-developer-platform/src/shared/errors"
)

func ValidateDeadlineBudget(ctx context.Context, margin time.Duration, connectTimeout time.Duration) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil
	}

	remaining := time.Until(deadline)
	if remaining-margin < connectTimeout {
		return errors.NewDeadlineExceededError("insufficient deadline budget for downstream call")
	}

	return nil
}
