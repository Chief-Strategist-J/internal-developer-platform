/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING LEDGER ADAPTERS (V.3 role 11)

 1. Role:
    - Persists INV-01 intent-before-effect records and outcomes in self_service_provisioning.provisioning_intents.
 2. Invariants:
    - Uses RunTracedClient / WithClientSpan (SpanKindClient) and emits structured error logs on DB failures.
    - Zero inline comments below this header.
*/
package adapters

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type PostgresProvisioningLedgerAdapter struct {
	db *sql.DB
}

func NewPostgresProvisioningLedgerAdapter(db *sql.DB) *PostgresProvisioningLedgerAdapter {
	return &PostgresProvisioningLedgerAdapter{db: db}
}

func (a *PostgresProvisioningLedgerAdapter) RecordIntent(ctx context.Context, intent contracts.ProvisioningIntent) error {
	return observability.WithClientSpan(ctx, "self_service.ledger.postgres.record_intent", func(ctx context.Context) error {
		logger := observability.GetLogger()
		if a.db == nil {
			err := errors.NewInternalError("provisioning ledger database connection is nil")
			logger.Error(ctx, "postgres provisioning intent insert failed: nil db", observability.String("error", err.Error()))
			return err
		}
		const query = `
			INSERT INTO self_service_provisioning.provisioning_intents (
				id, public_id, tenant_id, resource_kind, target_id, attempt, status, outcome_id, error_message
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (id) DO UPDATE SET
				attempt = EXCLUDED.attempt,
				status = EXCLUDED.status,
				updated_at = NOW()
		`
		_, err := a.db.ExecContext(
			ctx,
			query,
			intent.ID,
			intent.PublicID,
			intent.TenantID,
			intent.ResourceKind,
			intent.TargetID,
			intent.Attempt,
			intent.Status,
			intent.OutcomeID,
			intent.ErrorMessage,
		)
		if err != nil {
			logger.Error(ctx, "postgres provisioning intent insert failed",
				observability.String("db.system", "postgresql"),
				observability.String("intent_id", intent.ID),
				observability.String("error", err.Error()),
			)
			return fmt.Errorf("failed to record provisioning intent: %w", err)
		}
		return nil
	},
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "INSERT"),
		attribute.String("db.sql.table", "self_service_provisioning.provisioning_intents"),
		attribute.String("intent.id", intent.ID),
		attribute.String("resource.kind", intent.ResourceKind),
	)
}

func (a *PostgresProvisioningLedgerAdapter) RecordOutcome(ctx context.Context, intentID string, attempt int, status string, outcomeID string, errMsg string) error {
	return observability.WithClientSpan(ctx, "self_service.ledger.postgres.record_outcome", func(ctx context.Context) error {
		logger := observability.GetLogger()
		if a.db == nil {
			err := errors.NewInternalError("provisioning ledger database connection is nil")
			logger.Error(ctx, "postgres provisioning outcome update failed: nil db", observability.String("error", err.Error()))
			return err
		}
		const query = `
			UPDATE self_service_provisioning.provisioning_intents
			SET status = $3, outcome_id = $4, error_message = $5, version = version + 1, updated_at = NOW()
			WHERE id = $1 AND attempt = $2
		`
		res, err := a.db.ExecContext(ctx, query, intentID, attempt, status, outcomeID, errMsg)
		if err != nil {
			logger.Error(ctx, "postgres provisioning outcome update failed",
				observability.String("db.system", "postgresql"),
				observability.String("intent_id", intentID),
				observability.String("error", err.Error()),
			)
			return fmt.Errorf("failed to record provisioning outcome: %w", err)
		}
		if rows, _ := res.RowsAffected(); rows == 0 {
			conflictErr := errors.NewConflictError("stale attempt fencing conflict on provisioning intent")
			logger.Warn(ctx, "stale attempt rejected on provisioning outcome",
				observability.String("intent_id", intentID),
				observability.Int("attempt", attempt),
			)
			return conflictErr
		}
		return nil
	},
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "UPDATE"),
		attribute.String("db.sql.table", "self_service_provisioning.provisioning_intents"),
		attribute.String("intent.id", intentID),
		attribute.Int("intent.attempt", attempt),
	)
}

func (a *PostgresProvisioningLedgerAdapter) ListIntents(ctx context.Context, tenantID string, limit int) ([]contracts.ProvisioningIntent, error) {
	return observability.RunTracedClient(ctx, "self_service.ledger.postgres.list_intents", func(ctx context.Context) ([]contracts.ProvisioningIntent, error) {
		logger := observability.GetLogger()
		if a.db == nil {
			return nil, errors.NewInternalError("provisioning ledger database connection is nil")
		}
		if limit <= 0 {
			limit = 50
		}
		const query = `
			SELECT id, public_id, tenant_id, resource_kind, target_id, attempt, status, outcome_id, error_message
			FROM self_service_provisioning.provisioning_intents
			WHERE tenant_id = $1
			ORDER BY created_at DESC
			LIMIT $2
		`
		rows, err := a.db.QueryContext(ctx, query, tenantID, limit)
		if err != nil {
			logger.Error(ctx, "postgres provisioning intents query failed",
				observability.String("db.system", "postgresql"),
				observability.String("error", err.Error()),
			)
			return nil, err
		}
		defer rows.Close()

		items := make([]contracts.ProvisioningIntent, 0)
		for rows.Next() {
			var it contracts.ProvisioningIntent
			if err := rows.Scan(&it.ID, &it.PublicID, &it.TenantID, &it.ResourceKind, &it.TargetID, &it.Attempt, &it.Status, &it.OutcomeID, &it.ErrorMessage); err != nil {
				return nil, err
			}
			items = append(items, it)
		}
		return items, rows.Err()
	},
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "SELECT"),
		attribute.String("db.sql.table", "self_service_provisioning.provisioning_intents"),
	)
}

type MemoryProvisioningLedgerAdapter struct {
	mu      sync.Mutex
	Intents map[string]contracts.ProvisioningIntent
}

func NewMemoryProvisioningLedgerAdapter() *MemoryProvisioningLedgerAdapter {
	return &MemoryProvisioningLedgerAdapter{
		Intents: make(map[string]contracts.ProvisioningIntent),
	}
}

func (a *MemoryProvisioningLedgerAdapter) RecordIntent(ctx context.Context, intent contracts.ProvisioningIntent) error {
	return observability.WithClientSpan(ctx, "self_service.ledger.memory.record_intent", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.Intents[intent.ID] = intent
		return nil
	}, attribute.String("db.system", "memory"), attribute.String("intent.id", intent.ID))
}

func (a *MemoryProvisioningLedgerAdapter) RecordOutcome(ctx context.Context, intentID string, attempt int, status string, outcomeID string, errMsg string) error {
	return observability.WithClientSpan(ctx, "self_service.ledger.memory.record_outcome", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		it, ok := a.Intents[intentID]
		if !ok || it.Attempt != attempt {
			return errors.NewConflictError("stale attempt fencing conflict on provisioning intent")
		}
		it.Status = status
		it.OutcomeID = outcomeID
		it.ErrorMessage = errMsg
		a.Intents[intentID] = it
		return nil
	}, attribute.String("db.system", "memory"), attribute.String("intent.id", intentID))
}

func (a *MemoryProvisioningLedgerAdapter) ListIntents(ctx context.Context, tenantID string, limit int) ([]contracts.ProvisioningIntent, error) {
	return observability.RunTracedClient(ctx, "self_service.ledger.memory.list_intents", func(ctx context.Context) ([]contracts.ProvisioningIntent, error) {
		a.mu.Lock()
		defer a.mu.Unlock()
		items := make([]contracts.ProvisioningIntent, 0)
		for _, it := range a.Intents {
			if it.TenantID == tenantID {
				items = append(items, it)
				if limit > 0 && len(items) >= limit {
					break
				}
			}
		}
		return items, nil
	}, attribute.String("db.system", "memory"))
}
