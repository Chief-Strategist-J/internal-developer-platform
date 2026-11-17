package resilience

import (
	"context"
	"crypto/rand"
	"math"
	"math/big"
	"time"

	"internal-developer-platform/src/shared/errors"
)

type ResilienceExecutor struct {
	bulkhead       *Bulkhead
	circuitBreaker *CircuitBreaker
	retryBudget    *RetryBudget
	config         ResilienceConfig
}

func NewResilienceExecutor(config ResilienceConfig) *ResilienceExecutor {
	return &ResilienceExecutor{
		bulkhead:       NewBulkhead(config.MaxConcurrentRequests),
		circuitBreaker: NewCircuitBreaker(config),
		retryBudget:    NewRetryBudget(config.RetryMaxBudgetRatio, 100.0),
		config:         config,
	}
}

func (e *ResilienceExecutor) Execute(ctx context.Context, isIdempotent bool, operation func(ctx context.Context) error) error {
	if err := ValidateDeadlineBudget(ctx, e.config.MinDeadlineMargin, e.config.ConnectTimeout); err != nil {
		return err
	}

	if !e.circuitBreaker.Allow() {
		return errors.NewDownstreamUnavailableError("circuit breaker open for downstream call")
	}

	release, acquired := e.bulkhead.TryAcquire()
	if !acquired {
		return errors.NewDownstreamRateLimitedError("bulkhead capacity saturated for downstream call")
	}
	defer release()

	e.retryBudget.RecordRequest()

	var lastErr error
	maxAttempts := 1
	if isIdempotent {
		maxAttempts = e.config.MaxRetries
		if maxAttempts < 1 {
			maxAttempts = 1
		}
	}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if !e.retryBudget.TryAcquireRetryToken() {
				break
			}
			if err := ValidateDeadlineBudget(ctx, e.config.MinDeadlineMargin, e.config.ConnectTimeout); err != nil {
				return err
			}
			backoff := e.calculateBackoff(attempt)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return errors.NewDeadlineExceededError("operation context expired during backoff")
			}
		}

		err := operation(ctx)
		if err == nil {
			e.circuitBreaker.RecordSuccess()
			return nil
		}

		lastErr = err

		if !e.isRetryable(err) || !isIdempotent {
			e.circuitBreaker.RecordFailure()
			return err
		}
	}

	e.circuitBreaker.RecordFailure()
	return lastErr
}

func (e *ResilienceExecutor) isRetryable(err error) bool {
	if appErr, ok := err.(*errors.ApplicationError); ok {
		return appErr.Retryable
	}
	return false
}

func (e *ResilienceExecutor) calculateBackoff(attempt int) time.Duration {
	maxMs := float64(e.config.BackoffMax.Milliseconds())
	baseMs := float64(e.config.BackoffBase.Milliseconds())
	calculatedMs := math.Min(maxMs, baseMs*math.Pow(2, float64(attempt)))

	if calculatedMs <= 0 {
		return e.config.BackoffBase
	}

	n, randErr := rand.Int(rand.Reader, big.NewInt(int64(calculatedMs)))
	if randErr != nil {
		return time.Duration(calculatedMs) * time.Millisecond
	}

	return time.Duration(n.Int64()) * time.Millisecond
}

func (e *ResilienceExecutor) Bulkhead() *Bulkhead {
	return e.bulkhead
}

func (e *ResilienceExecutor) CircuitBreaker() *CircuitBreaker {
	return e.circuitBreaker
}

func (e *ResilienceExecutor) RetryBudget() *RetryBudget {
	return e.retryBudget
}

