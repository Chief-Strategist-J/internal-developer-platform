/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC AUDIT OUTBOX ADAPTERS

1. Architecture Role:
  - Implements ports.RBACAuditOutboxPort for durable RBAC decision audit logging.
  - PostgresRBACAuditOutboxAdapter persists PERMIT, DENY, and ELEVATED_PERMIT events into rbac_audit_outbox.
  - MemoryRBACAuditOutboxAdapter provides a thread-safe in-memory outbox for unit tests and non-DB mode.
  - Enforces Tier-4 OpenTelemetry spans and zero inline comments inside function bodies.
*/
package adapters

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	rbacQueries "internal-developer-platform/src/features/role-based-access-control/infrastructure/queries"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type PostgresRBACAuditOutboxAdapter struct {
	db *sql.DB
}

func NewPostgresRBACAuditOutboxAdapter(db *sql.DB) *PostgresRBACAuditOutboxAdapter {
	return &PostgresRBACAuditOutboxAdapter{db: db}
}

func (a *PostgresRBACAuditOutboxAdapter) EnqueueAuditEvent(ctx context.Context, event rbacContracts.RBACAuditEvent) error {
	return observability.WithSpan(ctx, "postgres.rbac_audit_outbox.enqueue", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("database connection is nil for rbac audit outbox")
		}
		if event.ID == "" {
			return errors.NewValidationError("audit event id is required")
		}
		occurredAt := event.OccurredAt
		if occurredAt.IsZero() {
			occurredAt = time.Now().UTC()
		}
		_, err := a.db.ExecContext(
			ctx,
			rbacQueries.FlowInsertRBACAuditOutbox,
			event.ID,
			event.TenantID,
			event.SubjectID,
			string(event.Resource),
			string(event.Action),
			event.Decision,
			event.Reason,
			event.ViolationCode,
			occurredAt,
		)
		if err != nil {
			return fmt.Errorf("failed to enqueue rbac audit outbox event: %w", err)
		}
		return nil
	}, attribute.String("event.id", event.ID), attribute.String("decision", event.Decision), attribute.String("subject.id", event.SubjectID))
}

type MemoryRBACAuditOutboxAdapter struct {
	mu     sync.RWMutex
	events []rbacContracts.RBACAuditEvent
}

func NewMemoryRBACAuditOutboxAdapter() *MemoryRBACAuditOutboxAdapter {
	return &MemoryRBACAuditOutboxAdapter{
		events: make([]rbacContracts.RBACAuditEvent, 0),
	}
}

func (a *MemoryRBACAuditOutboxAdapter) EnqueueAuditEvent(ctx context.Context, event rbacContracts.RBACAuditEvent) error {
	return observability.WithSpan(ctx, "memory.rbac_audit_outbox.enqueue", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.events = append(a.events, event)
		return nil
	}, attribute.String("event.id", event.ID), attribute.String("decision", event.Decision))
}

func (a *MemoryRBACAuditOutboxAdapter) ListEvents() []rbacContracts.RBACAuditEvent {
	a.mu.RLock()
	defer a.mu.RUnlock()
	copied := make([]rbacContracts.RBACAuditEvent, len(a.events))
	copy(copied, a.events)
	return copied
}
