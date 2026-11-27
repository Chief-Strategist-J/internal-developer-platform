/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC AUDIT OUTBOX RELAY LIFECYCLE

 1. Role (V.3 role 16, G-11, L-04, D-03):
    - Polls unpublished rows from rbac_audit_outbox and dispatches SIEM security alerts.
    - Marks published events upon downstream acknowledgement.
 2. Invariants:
    - Managed by the composition root Lifecycle registry rather than ad-hoc goroutines in BuildServices.
    - Zero mid-function inline comments.
*/
package lifecycle

import (
	"context"
	"sync/atomic"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/features/role-based-access-control/infrastructure/ports"
)

const defaultAuditRelayInterval = 5 * time.Second
const defaultAuditRelayBatchSize = 50

type AuditOutboxReaderPort interface {
	ListPendingAuditEvents(ctx context.Context, limit int) ([]rbacContracts.RBACAuditEvent, error)
	MarkAuditEventPublished(ctx context.Context, id string) error
}

type AuditOutboxRelayLifecycle struct {
	outbox   AuditOutboxReaderPort
	emitter  ports.SIEMAlertEmitterPort
	interval time.Duration
	ready    atomic.Bool
	cancel   context.CancelFunc
	done     chan struct{}
}

func NewAuditOutboxRelayLifecycle(outbox AuditOutboxReaderPort, emitter ports.SIEMAlertEmitterPort, interval time.Duration) *AuditOutboxRelayLifecycle {
	if interval <= 0 {
		interval = defaultAuditRelayInterval
	}
	return &AuditOutboxRelayLifecycle{
		outbox:   outbox,
		emitter:  emitter,
		interval: interval,
		done:     make(chan struct{}),
	}
}

func (l *AuditOutboxRelayLifecycle) Name() string {
	return "rbac.audit-outbox-relay"
}

func (l *AuditOutboxRelayLifecycle) Start(ctx context.Context) error {
	child, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	go l.run(child)
	return nil
}

func (l *AuditOutboxRelayLifecycle) Stop(ctx context.Context) error {
	if l.cancel != nil {
		l.cancel()
	}
	select {
	case <-l.done:
	case <-ctx.Done():
	}
	return nil
}

func (l *AuditOutboxRelayLifecycle) Ready() bool {
	return l.ready.Load()
}

func (l *AuditOutboxRelayLifecycle) run(ctx context.Context) {
	defer close(l.done)
	ticker := time.NewTicker(l.interval)
	defer ticker.Stop()
	l.ready.Store(true)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.relayBatch(ctx)
		}
	}
}

func (l *AuditOutboxRelayLifecycle) relayBatch(ctx context.Context) {
	if l.outbox == nil || l.emitter == nil {
		return
	}
	events, err := l.outbox.ListPendingAuditEvents(ctx, defaultAuditRelayBatchSize)
	if err != nil {
		return
	}
	for _, ev := range events {
		if err := l.emitter.EmitSecurityAlert(ctx, ev); err != nil {
			continue
		}
		_ = l.outbox.MarkAuditEventPublished(ctx, ev.ID)
	}
}
