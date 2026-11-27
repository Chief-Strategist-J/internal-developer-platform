/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC STREAMING SESSION REAPER LIFECYCLE

 1. Role (V.3 role 16, G-11, L-04):
    - Runs periodic sweeps to close streaming terminal sessions whose JWT or heartbeat window expired.
 2. Invariants:
    - Ticker lives strictly in lifecycle, never in RBACService (G-11).
    - Zero mid-function inline comments.
*/
package lifecycle

import (
	"context"
	"sync/atomic"
	"time"

	"internal-developer-platform/src/features/role-based-access-control/internal/service"
)

const defaultReaperInterval = 60 * time.Second

type StreamingReaperLifecycle struct {
	svc      *service.RBACService
	interval time.Duration
	ready    atomic.Bool
	cancel   context.CancelFunc
	done     chan struct{}
}

func NewStreamingReaperLifecycle(svc *service.RBACService, interval time.Duration) *StreamingReaperLifecycle {
	if interval <= 0 {
		interval = defaultReaperInterval
	}
	return &StreamingReaperLifecycle{
		svc:      svc,
		interval: interval,
		done:     make(chan struct{}),
	}
}

func (l *StreamingReaperLifecycle) Name() string {
	return "rbac.streaming-reaper"
}

func (l *StreamingReaperLifecycle) Start(ctx context.Context) error {
	child, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	go l.run(child)
	return nil
}

func (l *StreamingReaperLifecycle) Stop(ctx context.Context) error {
	if l.cancel != nil {
		l.cancel()
	}
	select {
	case <-l.done:
	case <-ctx.Done():
	}
	return nil
}

func (l *StreamingReaperLifecycle) Ready() bool {
	return l.ready.Load()
}

func (l *StreamingReaperLifecycle) run(ctx context.Context) {
	defer close(l.done)
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()
	l.ready.Store(true)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = l.svc.ReapExpiredStreamingSessions(ctx)
		}
	}
}
