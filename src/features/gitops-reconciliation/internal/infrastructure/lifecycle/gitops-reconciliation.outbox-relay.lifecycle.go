/*
ALGORITHM & ARCHITECTURE BLUEPRINT: GITOPS OUTBOX RELAY LIFECYCLE

 1. Role (V.3 role 16, G-11, D-03):
    - Runs periodic polling of unpublished GitOps outbox events.
 2. Invariants:
    - Timer runs inside Lifecycle.Start(), never inside constructors or BuildServices (G-11).
    - Zero mid-function inline comments.
*/
package lifecycle

import (
	"context"
	"sync/atomic"
	"time"

	"internal-developer-platform/src/features/gitops-reconciliation/internal/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/internal/service"
	"internal-developer-platform/src/infra/observability"
)

const defaultGitOpsRelayInterval = 5 * time.Second
const defaultGitOpsRelayBatchSize = 50

type GitOpsOutboxRelayLifecycle struct {
	svc      *service.GitOpsReconciliationService
	interval time.Duration
	ready    atomic.Bool
	cancel   context.CancelFunc
	done     chan struct{}
}

func NewGitOpsOutboxRelayLifecycle(svc *service.GitOpsReconciliationService, interval time.Duration) *GitOpsOutboxRelayLifecycle {
	if interval <= 0 {
		interval = defaultGitOpsRelayInterval
	}
	return &GitOpsOutboxRelayLifecycle{
		svc:      svc,
		interval: interval,
		done:     make(chan struct{}),
	}
}

func (l *GitOpsOutboxRelayLifecycle) Name() string {
	return "gitops.outbox-relay"
}

func (l *GitOpsOutboxRelayLifecycle) Start(ctx context.Context) error {
	child, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	go l.run(child)
	return nil
}

func (l *GitOpsOutboxRelayLifecycle) Stop(ctx context.Context) error {
	if l.cancel != nil {
		l.cancel()
	}
	select {
	case <-l.done:
	case <-ctx.Done():
	}
	return nil
}

func (l *GitOpsOutboxRelayLifecycle) Ready() bool {
	return l.ready.Load()
}

func (l *GitOpsOutboxRelayLifecycle) run(ctx context.Context) {
	defer close(l.done)
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()
	l.ready.Store(true)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.relayOnce(ctx)
		}
	}
}

func (l *GitOpsOutboxRelayLifecycle) relayOnce(ctx context.Context) {
	if l.svc == nil {
		return
	}
	_, err := l.svc.RelayPendingOutboxEvents(ctx, defaultGitOpsRelayBatchSize, func(pubCtx context.Context, ev contracts.OutboxEvent) error {
		observability.GetLogger().Info(pubCtx, "gitops outbox event dispatched",
			observability.String("event_id", ev.ID),
			observability.String("event_type", string(ev.EventType)),
			observability.String("aggregate_id", ev.AggregateID),
		)
		return nil
	})
	if err != nil {
		observability.GetLogger().Error(ctx, "gitops outbox relay cycle failed",
			observability.String("error", err.Error()),
		)
	}
}
