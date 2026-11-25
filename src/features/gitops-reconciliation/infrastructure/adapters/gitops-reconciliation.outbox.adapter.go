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

func (a *PostgresOutboxAdapter) FetchUnpublished(ctx context.Context, limit int) ([]contracts.OutboxEvent, error) {
	return observability.RunTraced(ctx, "gitops.outbox.postgres.fetch_unpublished", func(ctx context.Context) ([]contracts.OutboxEvent, error) {
		if a.db == nil {
			return nil, errors.NewInternalError("outbox database connection is nil")
		}
		if limit <= 0 {
			limit = 50
		}
		const query = `
			SELECT id, tenant_id, aggregate_id, event_type, app_name, revision, occurred_at
			FROM gitops_outbox_events
			WHERE published_at IS NULL
			ORDER BY occurred_at ASC
			LIMIT $1
		`
		rows, err := a.db.QueryContext(ctx, query, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		events := make([]contracts.OutboxEvent, 0)
		for rows.Next() {
			var ev contracts.OutboxEvent
			var evType string
			if err := rows.Scan(&ev.ID, &ev.TenantID, &ev.AggregateID, &evType, &ev.AppName, &ev.Revision, &ev.OccurredAt); err != nil {
				return nil, err
			}
			ev.EventType = contracts.OutboxEventType(evType)
			events = append(events, ev)
		}
		return events, rows.Err()
	})
}

func (a *PostgresOutboxAdapter) MarkPublished(ctx context.Context, eventID string) error {
	return observability.WithSpan(ctx, "gitops.outbox.postgres.mark_published", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("outbox database connection is nil")
		}
		const query = `UPDATE gitops_outbox_events SET published_at = NOW() WHERE id = $1 AND published_at IS NULL`
		_, err := a.db.ExecContext(ctx, query, eventID)
		return err
	}, attribute.String("outbox.event_id", eventID))
}

type MemoryOutboxAdapter struct {
	mu        sync.Mutex
	Events    []contracts.OutboxEvent
	Published map[string]bool
}

func NewMemoryOutboxAdapter() *MemoryOutboxAdapter {
	return &MemoryOutboxAdapter{
		Events:    make([]contracts.OutboxEvent, 0),
		Published: make(map[string]bool),
	}
}

func (a *MemoryOutboxAdapter) Enqueue(ctx context.Context, event contracts.OutboxEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Events = append(a.Events, event)
	return nil
}

func (a *MemoryOutboxAdapter) FetchUnpublished(ctx context.Context, limit int) ([]contracts.OutboxEvent, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if limit <= 0 {
		limit = 50
	}
	res := make([]contracts.OutboxEvent, 0)
	for _, ev := range a.Events {
		if !a.Published[ev.ID] {
			res = append(res, ev)
			if len(res) >= limit {
				break
			}
		}
	}
	return res, nil
}

func (a *MemoryOutboxAdapter) MarkPublished(ctx context.Context, eventID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Published[eventID] = true
	return nil
}
