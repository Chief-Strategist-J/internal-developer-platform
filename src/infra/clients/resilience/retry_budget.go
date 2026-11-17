package resilience

import (
	"sync"
)

type RetryBudget struct {
	mu           sync.Mutex
	ratioMillts  int64
	milliTokens  int64
	maxMilliTkns int64
	requestCount uint64
	retryCount   uint64
}

func NewRetryBudget(ratio float64, maxTokens float64) *RetryBudget {
	if ratio <= 0.0 {
		ratio = 0.10
	}
	if maxTokens <= 0.0 {
		maxTokens = 100.0
	}
	rMilli := int64(ratio * 1000.0)
	if rMilli <= 0 {
		rMilli = 100
	}
	maxMilli := int64(maxTokens * 1000.0)
	return &RetryBudget{
		ratioMillts:  rMilli,
		milliTokens:  10 * 1000,
		maxMilliTkns: maxMilli,
	}
}

func (rb *RetryBudget) RecordRequest() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.requestCount++
	rb.milliTokens += rb.ratioMillts
	if rb.milliTokens > rb.maxMilliTkns {
		rb.milliTokens = rb.maxMilliTkns
	}
}

func (rb *RetryBudget) TryAcquireRetryToken() bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.milliTokens >= 1000 {
		rb.milliTokens -= 1000
		rb.retryCount++
		return true
	}
	return false
}

func (rb *RetryBudget) AvailableTokens() float64 {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return float64(rb.milliTokens) / 1000.0
}
