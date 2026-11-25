// Package adapters provides the concrete PostgreSQL and in-memory outbox adapters for GitOps Reconciliation.
package adapters

import (
	"context"
	"database/sql"
	"sync"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type PostgresOutboxAdapter struct {
	db *sql.DB
}

func NewPostgresOutboxAdapter(db *sql.DB) *PostgresOutboxAdapter {
	return &PostgresOutboxAdapter{db: db}
}

func (a *PostgresOutboxAdapter) Enqueue(ctx context.Context, event contracts.OutboxEvent) error {
	return observability.WithSpan(ctx, "gitops.outbox.postgres.enqueue", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("outbox database connection is nil")
		}
		const query = `
			INSERT INTO gitops_outbox_events (id, tenant_id, aggregate_id, event_type, app_name, revision, occurred_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`
		_, err := a.db.ExecContext(
			ctx,
			query,
			event.ID,
			event.TenantID,
			event.AggregateID,
			string(event.EventType),
			event.AppName,
			event.Revision,
			event.OccurredAt,
		)
		return err
	}, attribute.String("outbox.event_type", string(event.EventType)), attribute.String("aggregate.id", event.AggregateID))
}

type MemoryOutboxAdapter struct {
	mu     sync.Mutex
	Events []contracts.OutboxEvent
}

func NewMemoryOutboxAdapter() *MemoryOutboxAdapter {
	return &MemoryOutboxAdapter{
		Events: make([]contracts.OutboxEvent, 0),
	}
}

func (a *MemoryOutboxAdapter) Enqueue(ctx context.Context, event contracts.OutboxEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Events = append(a.Events, event)
	return nil
}
