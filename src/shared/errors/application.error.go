package errors

import stdErrors "errors"

type ApplicationError struct {
	StatusCode    int
	CanonicalCode string
	Message       string
	Retryable     bool
}

func (e *ApplicationError) Error() string {
	return e.Message
}

func NewValidationError(message string) *ApplicationError {
	return &ApplicationError{
		StatusCode:    400,
		CanonicalCode: "VALIDATION_FAILED",
		Message:       message,
		Retryable:     false,
	}
}

func NewNotFoundError(message string) *ApplicationError {
	return &ApplicationError{
		StatusCode:    404,
		CanonicalCode: "RESOURCE_NOT_FOUND",
		Message:       message,
		Retryable:     false,
	}
}

func NewConflictError(message string) *ApplicationError {
	return &ApplicationError{
		StatusCode:    409,
		CanonicalCode: "CONFLICT",
		Message:       message,
		Retryable:     false,
	}
}

func NewUnauthorizedError(message string) *ApplicationError {
	return &ApplicationError{
		StatusCode:    401,
		CanonicalCode: "UNAUTHORIZED",
		Message:       message,
		Retryable:     false,
	}
}

func NewForbiddenError(message string) *ApplicationError {
	return &ApplicationError{
		StatusCode:    403,
		CanonicalCode: "FORBIDDEN",
		Message:       message,
		Retryable:     false,
	}
}

func NewDownstreamUnavailableError(message string) *ApplicationError {
	return &ApplicationError{
		StatusCode:    503,
		CanonicalCode: "DOWNSTREAM_UNAVAILABLE",
		Message:       message,
		Retryable:     true,
	}
}

func NewDownstreamRateLimitedError(message string) *ApplicationError {
	return &ApplicationError{
		StatusCode:    429,
		CanonicalCode: "DOWNSTREAM_RATE_LIMITED",
		Message:       message,
		Retryable:     true,
	}
}

func NewDeadlineExceededError(message string) *ApplicationError {
	return &ApplicationError{
		StatusCode:    504,
		CanonicalCode: "DEADLINE_EXCEEDED",
		Message:       message,
		Retryable:     false,
	}
}

func NewInternalError(message string, cause ...error) *ApplicationError {
	errMsg := message
	if len(cause) > 0 && cause[0] != nil {
		errMsg = message + ": " + cause[0].Error()
	}
	return &ApplicationError{
		StatusCode:    500,
		CanonicalCode: "INTERNAL_ERROR",
		Message:       errMsg,
		Retryable:     false,
	}
}

func IsConflictError(err error) bool {
	var appErr *ApplicationError
	if stdErrors.As(err, &appErr) {
		return appErr.StatusCode == 409
	}
	return false
}

func IsNotFoundError(err error) bool {
	var appErr *ApplicationError
	if stdErrors.As(err, &appErr) {
		return appErr.StatusCode == 404
	}
	return false
}

func IsUnauthorizedError(err error) bool {
	var appErr *ApplicationError
	if stdErrors.As(err, &appErr) {
		return appErr.StatusCode == 401
	}
	return false
}

func IsForbiddenError(err error) bool {
	var appErr *ApplicationError
	if stdErrors.As(err, &appErr) {
		return appErr.StatusCode == 403
	}
	return false
}
