package resilience

import (
	"context"
	"errors"
	"testing"
	"time"

	apperrors "internal-developer-platform/src/shared/errors"
)

func TestBulkheadSaturation(t *testing.T) {
	bulkhead := NewBulkhead(2)

	rel1, ok1 := bulkhead.TryAcquire()
	if !ok1 {
		t.Fatalf("expected first acquisition to succeed")
	}
	defer rel1()

	rel2, ok2 := bulkhead.TryAcquire()
	if !ok2 {
		t.Fatalf("expected second acquisition to succeed")
	}
	defer rel2()

	_, ok3 := bulkhead.TryAcquire()
	if ok3 {
		t.Fatalf("expected third acquisition to be rejected by bulkhead")
	}

	rel2()

	rel4, ok4 := bulkhead.TryAcquire()
	if !ok4 {
		t.Fatalf("expected acquisition to succeed after permit release")
	}
	defer rel4()
}

func TestCircuitBreakerLifecycle(t *testing.T) {
	cfg := DefaultResilienceConfig()
	cfg.FailureThreshold = 2
	cfg.RecoveryTimeout = 50 * time.Millisecond
	cfg.HalfOpenMaxRequests = 1

	cb := NewCircuitBreaker(cfg)

	if !cb.Allow() {
		t.Fatalf("expected closed circuit to allow calls")
	}

	cb.RecordFailure()
	if cb.State() != CircuitStateClosed {
		t.Fatalf("circuit should remain closed before threshold")
	}

	cb.RecordFailure()
	if cb.State() != CircuitStateOpen {
		t.Fatalf("circuit should open after threshold reached")
	}

	if cb.Allow() {
		t.Fatalf("expected open circuit to reject calls")
	}

	time.Sleep(60 * time.Millisecond)

	if !cb.Allow() {
		t.Fatalf("expected circuit to allow probe in half-open state")
	}
	if cb.State() != CircuitStateHalfOpen {
		t.Fatalf("circuit should be half-open")
	}

	cb.RecordSuccess()
	if cb.State() != CircuitStateClosed {
		t.Fatalf("circuit should close after successful probe")
	}
}

func TestDeadlineBudgetValidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := ValidateDeadlineBudget(ctx, 50*time.Millisecond, 100*time.Millisecond)
	if err == nil {
		t.Fatalf("expected deadline exceeded error when remaining budget is below margin")
	}

	validCtx, validCancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer validCancel()

	if err := ValidateDeadlineBudget(validCtx, 50*time.Millisecond, 100*time.Millisecond); err != nil {
		t.Fatalf("expected validation to pass when budget is ample: %v", err)
	}
}

func TestRetryBudgetThrottling(t *testing.T) {
	rb := NewRetryBudget(0.10, 5.0)

	for i := 0; i < 10; i++ {
		rb.TryAcquireRetryToken()
	}

	if rb.TryAcquireRetryToken() {
		t.Fatalf("expected retry token acquisition to fail when exhausted")
	}

	rb.RecordRequest()
	rb.RecordRequest()
	rb.RecordRequest()
	rb.RecordRequest()
	rb.RecordRequest()
	rb.RecordRequest()
	rb.RecordRequest()
	rb.RecordRequest()
	rb.RecordRequest()
	rb.RecordRequest()

	if !rb.TryAcquireRetryToken() {
		t.Fatalf("expected token available after 10 requests at 0.1 ratio")
	}
}

func TestResilienceExecutorIntegration(t *testing.T) {
	cfg := DefaultResilienceConfig()
	cfg.FailureThreshold = 2
	cfg.MaxRetries = 2
	cfg.BackoffBase = 1 * time.Millisecond
	cfg.BackoffMax = 5 * time.Millisecond

	exec := NewResilienceExecutor(cfg)

	t.Run("NonIdempotentDoesNotRetryOnFailure", func(t *testing.T) {
		attempts := 0
		err := exec.Execute(context.Background(), false, func(ctx context.Context) error {
			attempts++
			return apperrors.NewDownstreamUnavailableError("temporary fault")
		})
		if err == nil {
			t.Fatalf("expected error from downstream")
		}
		if attempts != 1 {
			t.Fatalf("expected exactly 1 attempt for non-idempotent operation, got %d", attempts)
		}
	})

	t.Run("IdempotentRetriesOnRetryableError", func(t *testing.T) {
		attempts := 0
		err := exec.Execute(context.Background(), true, func(ctx context.Context) error {
			attempts++
			if attempts < 2 {
				return apperrors.NewDownstreamUnavailableError("transient network blip")
			}
			return nil
		})
		if err != nil {
			t.Fatalf("expected success on retry: %v", err)
		}
		if attempts != 2 {
			t.Fatalf("expected 2 attempts, got %d", attempts)
		}
	})

	t.Run("NonRetryableErrorFailsFast", func(t *testing.T) {
		attempts := 0
		err := exec.Execute(context.Background(), true, func(ctx context.Context) error {
			attempts++
			return errors.New("permanent unrecoverable error")
		})
		if err == nil {
			t.Fatalf("expected error")
		}
		if attempts != 1 {
			t.Fatalf("expected 1 attempt for permanent error, got %d", attempts)
		}
	})
}

