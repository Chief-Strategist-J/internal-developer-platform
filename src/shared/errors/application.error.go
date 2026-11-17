package errors

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
