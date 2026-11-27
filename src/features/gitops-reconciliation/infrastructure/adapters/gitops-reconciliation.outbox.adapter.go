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
	return observability.WithClientSpan(ctx, "gitops.outbox.postgres.enqueue", func(ctx context.Context) error {
		logger := observability.GetLogger()
		if a.db == nil {
			err := errors.NewInternalError("outbox database connection is nil")
			logger.Error(ctx, "postgres outbox enqueue failed: nil db connection", observability.String("error", err.Error()))
			return err
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
		if err != nil {
			logger.Error(ctx, "postgres outbox enqueue query failed",
				observability.String("db.system", "postgresql"),
				observability.String("db.operation", "INSERT"),
				observability.String("error", err.Error()),
			)
		}
		return err
	},
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "INSERT"),
		attribute.String("db.sql.table", "gitops_outbox_events"),
		attribute.String("outbox.event_type", string(event.EventType)),
		attribute.String("aggregate.id", event.AggregateID),
	)
}

func (a *PostgresOutboxAdapter) FetchUnpublished(ctx context.Context, limit int) ([]contracts.OutboxEvent, error) {
	return observability.RunTracedClient(ctx, "gitops.outbox.postgres.fetch_unpublished", func(ctx context.Context) ([]contracts.OutboxEvent, error) {
		logger := observability.GetLogger()
		if a.db == nil {
			err := errors.NewInternalError("outbox database connection is nil")
			logger.Error(ctx, "postgres outbox fetch failed: nil db connection", observability.String("error", err.Error()))
			return nil, err
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
			logger.Error(ctx, "postgres outbox fetch query failed",
				observability.String("db.system", "postgresql"),
				observability.String("db.operation", "SELECT"),
				observability.String("error", err.Error()),
			)
			return nil, err
		}
		defer rows.Close()

		events := make([]contracts.OutboxEvent, 0)
		for rows.Next() {
			var ev contracts.OutboxEvent
			var evType string
			if err := rows.Scan(&ev.ID, &ev.TenantID, &ev.AggregateID, &evType, &ev.AppName, &ev.Revision, &ev.OccurredAt); err != nil {
				logger.Error(ctx, "postgres outbox row scan failed",
					observability.String("db.system", "postgresql"),
					observability.String("error", err.Error()),
				)
				return nil, err
			}
			ev.EventType = contracts.OutboxEventType(evType)
			events = append(events, ev)
		}
		if err := rows.Err(); err != nil {
			logger.Error(ctx, "postgres outbox rows iteration failed",
				observability.String("db.system", "postgresql"),
				observability.String("error", err.Error()),
			)
			return nil, err
		}
		observability.CurrentSpan(ctx).SetAttributes(attribute.Int("db.rows_returned", len(events)))
		return events, nil
	},
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "gitops_outbox_events"),
		attribute.Int("db.limit", limit),
	)
}

func (a *PostgresOutboxAdapter) MarkPublished(ctx context.Context, eventID string) error {
	return observability.WithClientSpan(ctx, "gitops.outbox.postgres.mark_published", func(ctx context.Context) error {
		logger := observability.GetLogger()
		if a.db == nil {
			err := errors.NewInternalError("outbox database connection is nil")
			logger.Error(ctx, "postgres outbox mark_published failed: nil db connection", observability.String("error", err.Error()))
			return err
		}
		const query = `UPDATE gitops_outbox_events SET published_at = NOW() WHERE id = $1 AND published_at IS NULL`
		_, err := a.db.ExecContext(ctx, query, eventID)
		if err != nil {
			logger.Error(ctx, "postgres outbox mark_published query failed",
				observability.String("db.system", "postgresql"),
				observability.String("db.operation", "UPDATE"),
				observability.String("outbox.event_id", eventID),
				observability.String("error", err.Error()),
			)
		}
		return err
	},
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "UPDATE"),
		attribute.String("db.sql.table", "gitops_outbox_events"),
		attribute.String("outbox.event_id", eventID),
	)
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
	return observability.WithClientSpan(ctx, "gitops.outbox.memory.enqueue", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.Events = append(a.Events, event)
		return nil
	},
		attribute.String("db.system", "memory"),
		attribute.String("outbox.event_id", event.ID),
		attribute.String("outbox.event_type", string(event.EventType)),
	)
}

func (a *MemoryOutboxAdapter) FetchUnpublished(ctx context.Context, limit int) ([]contracts.OutboxEvent, error) {
	return observability.RunTracedClient(ctx, "gitops.outbox.memory.fetch_unpublished", func(ctx context.Context) ([]contracts.OutboxEvent, error) {
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
		observability.CurrentSpan(ctx).SetAttributes(attribute.Int("db.rows_returned", len(res)))
		return res, nil
	},
		attribute.String("db.system", "memory"),
		attribute.Int("db.limit", limit),
	)
}

func (a *MemoryOutboxAdapter) MarkPublished(ctx context.Context, eventID string) error {
	return observability.WithClientSpan(ctx, "gitops.outbox.memory.mark_published", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.Published[eventID] = true
		return nil
	},
		attribute.String("db.system", "memory"),
		attribute.String("outbox.event_id", eventID),
	)
}
