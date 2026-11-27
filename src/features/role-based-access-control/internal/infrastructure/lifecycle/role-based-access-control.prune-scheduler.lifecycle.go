/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC LEAST-PRIVILEGE PRUNING SCHEDULER LIFECYCLE

 1. Role (V.3 role 16, G-11, L-04, XIII.3):
    - Owns the ticker loop for periodic least-privilege role analysis.
    - Acquires a distributed leader lease before running each cycle (context.yml lease: true).
 2. Invariants:
    - Constructors and services MUST NOT spawn goroutines or timers (G-11).
    - Zero inline comments below this header.
*/
package lifecycle

import (
	"context"
	"sync/atomic"
	"time"

	"internal-developer-platform/src/features/role-based-access-control/internal/service"
)

const defaultPruneInterval = 24 * time.Hour
const defaultPruneStaleWindow = 90 * 24 * time.Hour

type LeaderLeasePort interface {
	TryAcquireLeaderLease(ctx context.Context, leaseKey string, ttl time.Duration) (bool, error)
}

type PruneSchedulerLifecycle struct {
	svc       *service.RBACService
	leasePort LeaderLeasePort
	interval  time.Duration
	ready     atomic.Bool
	cancel    context.CancelFunc
	done      chan struct{}
}

func NewPruneSchedulerLifecycle(svc *service.RBACService, leasePort LeaderLeasePort, interval time.Duration) *PruneSchedulerLifecycle {
	if interval <= 0 {
		interval = defaultPruneInterval
	}
	return &PruneSchedulerLifecycle{
		svc:       svc,
		leasePort: leasePort,
		interval:  interval,
		done:      make(chan struct{}),
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
	l.runCycleWithLease(ctx)
	l.ready.Store(true)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.runCycleWithLease(ctx)
		}
	}
}

func (l *PruneSchedulerLifecycle) runCycleWithLease(ctx context.Context) {
	if l.leasePort != nil {
		acquired, err := l.leasePort.TryAcquireLeaderLease(ctx, l.Name(), l.interval)
		if err != nil || !acquired {
			return
		}
	}
	_, _ = l.svc.RunLeastPrivilegeAuditCycle(ctx, defaultPruneStaleWindow)
}
