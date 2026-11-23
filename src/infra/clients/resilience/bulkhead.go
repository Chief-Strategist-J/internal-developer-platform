package resilience

import (
	"sync"
)

type Bulkhead struct {
	mu        sync.Mutex
	semaphore chan struct{}
	capacity  int
}

func NewBulkhead(maxConcurrent int) *Bulkhead {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	return &Bulkhead{
		semaphore: make(chan struct{}, maxConcurrent),
		capacity:  maxConcurrent,
	}
}

func (b *Bulkhead) TryAcquire() (func(), bool) {
	select {
	case b.semaphore <- struct{}{}:
		var once sync.Once
		release := func() {
			once.Do(func() {
				<-b.semaphore
			})
		}
		return release, true
	default:
		return nil, false
	}
}

func (b *Bulkhead) Capacity() int {
	return b.capacity
}

func (b *Bulkhead) InFlight() int {
	return len(b.semaphore)
}
