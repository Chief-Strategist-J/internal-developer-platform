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

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/features/self-service-provisioning/internal/infrastructure/queries"
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
		_, err := a.db.ExecContext(
			ctx,
			queries.UpsertProvisioningIntentSQL,
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
		res, err := a.db.ExecContext(ctx, queries.UpdateProvisioningOutcomeFencedSQL, intentID, attempt, status, outcomeID, errMsg)
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
		rows, err := a.db.QueryContext(ctx, queries.ListProvisioningIntentsByTenantSQL, tenantID, limit)
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
