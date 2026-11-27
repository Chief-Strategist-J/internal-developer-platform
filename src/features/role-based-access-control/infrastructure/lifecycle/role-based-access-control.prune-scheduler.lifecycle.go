/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC LEAST-PRIVILEGE PRUNING SCHEDULER LIFECYCLE

 1. Role (V.3 role 16, G-11, L-04):
    - Owns the ticker loop for periodic least-privilege role analysis.
    - Start() launches the ticker; Stop() drains within deadline; Ready() reports completion.
 2. Invariants:
    - Constructors and services MUST NOT spawn goroutines or timers (G-11).
    - Zero mid-function inline comments.
*/
package lifecycle

import (
	"context"
	"sync/atomic"
	"time"

	"internal-developer-platform/src/features/role-based-access-control/service"
)

const defaultPruneInterval = 24 * time.Hour
const defaultPruneStaleWindow = 90 * 24 * time.Hour

type PruneSchedulerLifecycle struct {
	svc      *service.RBACService
	interval time.Duration
	ready    atomic.Bool
	cancel   context.CancelFunc
	done     chan struct{}
}

func NewPruneSchedulerLifecycle(svc *service.RBACService, interval time.Duration) *PruneSchedulerLifecycle {
	if interval <= 0 {
		interval = defaultPruneInterval
	}
	return &PruneSchedulerLifecycle{
		svc:      svc,
		interval: interval,
		done:     make(chan struct{}),
	}
}

func (l *PruneSchedulerLifecycle) Name() string {
	return "rbac.prune-scheduler"
}

func (l *PruneSchedulerLifecycle) Start(ctx context.Context) error {
	child, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	go l.run(child)
	return nil
}

func (l *PruneSchedulerLifecycle) Stop(ctx context.Context) error {
	if l.cancel != nil {
		l.cancel()
	}
	select {
	case <-l.done:
	case <-ctx.Done():
	}
	return nil
}

func (l *PruneSchedulerLifecycle) Ready() bool {
	return l.ready.Load()
}

func (l *PruneSchedulerLifecycle) run(ctx context.Context) {
	defer close(l.done)
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()
	_, _ = l.svc.RunLeastPrivilegeAuditCycle(ctx, defaultPruneStaleWindow)
	l.ready.Store(true)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = l.svc.RunLeastPrivilegeAuditCycle(ctx, defaultPruneStaleWindow)
		}
	}
}
