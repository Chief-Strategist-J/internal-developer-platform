/*
ALGORITHM & ARCHITECTURE BLUEPRINT: DISASTER RECOVERY PROBER LIFECYCLE (V.3 role 16, G-11)

 1. Role:
    - Owns the ticker loop for periodic DR readiness checks across active plans.
 2. Invariants:
    - Goroutines and timers live strictly in lifecycle, never in constructors or DisasterRecoveryService (G-11).
    - Zero inline or mid-file comments below this header.
*/
package lifecycle

import (
	"context"
	"sync/atomic"
	"time"

	"internal-developer-platform/src/features/disaster-recovery/internal/service"
)

const defaultProberInterval = 30 * time.Second

type ReadinessProberLifecycle struct {
	svc      *service.DisasterRecoveryService
	interval time.Duration
	ready    atomic.Bool
	cancel   context.CancelFunc
	done     chan struct{}
}

func NewReadinessProberLifecycle(svc *service.DisasterRecoveryService, interval time.Duration) *ReadinessProberLifecycle {
	if interval <= 0 {
		interval = defaultProberInterval
	}
	return &ReadinessProberLifecycle{
		svc:      svc,
		interval: interval,
		done:     make(chan struct{}),
	}
}

func (l *ReadinessProberLifecycle) Name() string {
	return "disaster-recovery.readiness-prober"
}

func (l *ReadinessProberLifecycle) Start(ctx context.Context) error {
	child, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	go l.run(child)
	return nil
}

func (l *ReadinessProberLifecycle) Stop(ctx context.Context) error {
	if l.cancel != nil {
		l.cancel()
	}
	select {
	case <-l.done:
	case <-ctx.Done():
	}
	return nil
}

func (l *ReadinessProberLifecycle) Ready() bool {
	return l.ready.Load()
}

func (l *ReadinessProberLifecycle) run(ctx context.Context) {
	defer close(l.done)
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()
	l.ready.Store(true)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = l.svc.ProbeAllPlansOnce(ctx)
		}
	}
}
