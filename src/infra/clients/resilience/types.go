package resilience

import (
	"time"
)

type CircuitState string

const (
	CircuitStateClosed   CircuitState = "CLOSED"
	CircuitStateOpen     CircuitState = "OPEN"
	CircuitStateHalfOpen CircuitState = "HALF_OPEN"
)

type ResilienceConfig struct {
	MaxConcurrentRequests int
	FailureThreshold      int
	RecoveryTimeout       time.Duration
	HalfOpenMaxRequests   int
	MinDeadlineMargin     time.Duration
	ConnectTimeout        time.Duration
	RetryMaxBudgetRatio   float64
	MaxRetries            int
	BackoffBase           time.Duration
	BackoffMax            time.Duration
}

func DefaultResilienceConfig() ResilienceConfig {
	return ResilienceConfig{
		MaxConcurrentRequests: 50,
		FailureThreshold:      5,
		RecoveryTimeout:       10 * time.Second,
		HalfOpenMaxRequests:   3,
		MinDeadlineMargin:     50 * time.Millisecond,
		ConnectTimeout:        100 * time.Millisecond,
		RetryMaxBudgetRatio:   0.10,
		MaxRetries:            3,
		BackoffBase:           20 * time.Millisecond,
		BackoffMax:            500 * time.Millisecond,
	}
}
