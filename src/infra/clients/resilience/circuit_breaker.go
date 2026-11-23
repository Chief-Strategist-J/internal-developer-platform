package resilience

import (
	"sync"
	"time"
)

type CircuitBreaker struct {
	mu                   sync.RWMutex
	state                CircuitState
	failureCount         int
	halfOpenSuccessCount int
	halfOpenAttempts     int
	lastStateChange      time.Time
	config               ResilienceConfig
}

func NewCircuitBreaker(config ResilienceConfig) *CircuitBreaker {
	return &CircuitBreaker{
		state:           CircuitStateClosed,
		lastStateChange: time.Now().UTC(),
		config:          config,
	}
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.evaluateStateChange()
	return cb.state
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.evaluateStateChange()

	if cb.state == CircuitStateClosed {
		return true
	}

	if cb.state == CircuitStateHalfOpen {
		if cb.halfOpenAttempts < cb.config.HalfOpenMaxRequests {
			cb.halfOpenAttempts++
			return true
		}
		return false
	}

	return false
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitStateHalfOpen {
		cb.halfOpenSuccessCount++
		if cb.halfOpenSuccessCount >= cb.config.HalfOpenMaxRequests {
			cb.state = CircuitStateClosed
			cb.failureCount = 0
			cb.halfOpenSuccessCount = 0
			cb.halfOpenAttempts = 0
			cb.lastStateChange = time.Now().UTC()
		}
		return
	}

	if cb.state == CircuitStateClosed {
		cb.failureCount = 0
	}
}

func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitStateHalfOpen {
		cb.state = CircuitStateOpen
		cb.failureCount = cb.config.FailureThreshold
		cb.halfOpenSuccessCount = 0
		cb.halfOpenAttempts = 0
		cb.lastStateChange = time.Now().UTC()
		return
	}

	if cb.state == CircuitStateClosed {
		cb.failureCount++
		if cb.failureCount >= cb.config.FailureThreshold {
			cb.state = CircuitStateOpen
			cb.lastStateChange = time.Now().UTC()
		}
	}
}

func (cb *CircuitBreaker) evaluateStateChange() {
	if cb.state == CircuitStateOpen {
		if time.Since(cb.lastStateChange) >= cb.config.RecoveryTimeout {
			cb.state = CircuitStateHalfOpen
			cb.halfOpenAttempts = 0
			cb.halfOpenSuccessCount = 0
			cb.lastStateChange = time.Now().UTC()
		}
	}
}
