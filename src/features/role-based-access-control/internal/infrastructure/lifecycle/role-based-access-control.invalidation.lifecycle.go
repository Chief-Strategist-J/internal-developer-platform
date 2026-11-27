/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC INVALIDATION SUBSCRIBER LIFECYCLE

 1. Role (V.3 role 16, L-04):
    - Subscribes to Redis pub/sub cache invalidation channel on Start().
 2. Invariants:
    - Connection/goroutine subscription starts during Lifecycle.Start(), never inside constructors.
    - Zero mid-function inline comments.
*/
package lifecycle

import (
	"context"
	"sync/atomic"
)

type InvalidationSubscriberPort interface {
	SubscribeInvalidation(ctx context.Context, onInvalidate func(subjectID string))
}

type InvalidationListenerLifecycle struct {
	sub          InvalidationSubscriberPort
	onInvalidate func(subjectID string)
	ready        atomic.Bool
	cancel       context.CancelFunc
}

func NewInvalidationListenerLifecycle(sub InvalidationSubscriberPort, onInvalidate func(subjectID string)) *InvalidationListenerLifecycle {
	return &InvalidationListenerLifecycle{
		sub:          sub,
		onInvalidate: onInvalidate,
	}
}

func (l *InvalidationListenerLifecycle) Name() string {
	return "rbac.invalidation-listener"
}

func (l *InvalidationListenerLifecycle) Start(ctx context.Context) error {
	child, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	if l.sub != nil {
		l.sub.SubscribeInvalidation(child, l.onInvalidate)
	}
	l.ready.Store(true)
	return nil
}

func (l *InvalidationListenerLifecycle) Stop(ctx context.Context) error {
	if l.cancel != nil {
		l.cancel()
	}
	l.ready.Store(false)
	return nil
}

func (l *InvalidationListenerLifecycle) Ready() bool {
	return l.ready.Load()
}
