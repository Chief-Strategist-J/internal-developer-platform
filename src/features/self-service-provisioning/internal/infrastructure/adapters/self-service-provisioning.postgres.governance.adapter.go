/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING GOVERNANCE ADAPTERS (V.3 role 11)

 1. Role:
    - Persists INV-02 operation IDs, INV-06 retention decisions, INV-07 DNS cooldowns, EC-13 saga locks, and Section C sweeper queries.
 2. Invariants:
    - Uses RunTracedClient / WithClientSpan (SpanKindClient); zero inline comments below this header.
*/
package adapters

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/features/self-service-provisioning/internal/domain/rules"
	"internal-developer-platform/src/features/self-service-provisioning/internal/infrastructure/queries"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type PostgresProvisioningGovernanceAdapter struct {
	db *sql.DB
}

func NewPostgresProvisioningGovernanceAdapter(db *sql.DB) *PostgresProvisioningGovernanceAdapter {
	return &PostgresProvisioningGovernanceAdapter{db: db}
}

func (a *PostgresProvisioningGovernanceAdapter) RecordOperationID(ctx context.Context, intentID string, operationID string) error {
	return observability.WithClientSpan(ctx, "self_service.governance.postgres.record_op", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("governance db is nil")
		}
		_, err := a.db.ExecContext(ctx, queries.UpdateIntentOperationIDSQL, intentID, operationID)
		return err
	}, attribute.String("db.system", "postgresql"), attribute.String("intent.id", intentID))
}

func (a *PostgresProvisioningGovernanceAdapter) RecordRetentionDecision(ctx context.Context, d contracts.RetentionDecision) error {
	return observability.WithClientSpan(ctx, "self_service.governance.postgres.record_retention", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("governance db is nil")
		}
		_, err := a.db.ExecContext(ctx, queries.InsertRetentionDecisionSQL, d.ID, d.TenantID, d.ResourceID, d.ResourceClass, d.Decision, d.DecidedBy)
		return err
	}, attribute.String("db.system", "postgresql"), attribute.String("resource.id", d.ResourceID))
}

func (a *PostgresProvisioningGovernanceAdapter) HasRetentionDecision(ctx context.Context, tenantID string, resourceID string) (bool, error) {
	return observability.RunTracedClient(ctx, "self_service.governance.postgres.has_retention", func(ctx context.Context) (bool, error) {
		if a.db == nil {
			return false, errors.NewInternalError("governance db is nil")
		}
		var count int
		if err := a.db.QueryRowContext(ctx, queries.CountRetentionDecisionSQL, tenantID, resourceID).Scan(&count); err != nil {
			return false, err
		}
		return count > 0, nil
	}, attribute.String("db.system", "postgresql"), attribute.String("resource.id", resourceID))
}

func (a *PostgresProvisioningGovernanceAdapter) EnterDNSCooldown(ctx context.Context, tenantID string, dnsName string, cooldown time.Duration) error {
	return observability.WithClientSpan(ctx, "self_service.governance.postgres.dns_cooldown", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("governance db is nil")
		}
		norm := rules.NormalizeResourceName(dnsName)
		until := time.Now().Add(cooldown)
		_, err := a.db.ExecContext(ctx, queries.UpsertDNSCooldownSQL, norm, tenantID, until)
		return err
	}, attribute.String("db.system", "postgresql"), attribute.String("dns.name", dnsName))
}

func (a *PostgresProvisioningGovernanceAdapter) IsDNSInCooldown(ctx context.Context, dnsName string) (bool, error) {
	return observability.RunTracedClient(ctx, "self_service.governance.postgres.check_dns_cooldown", func(ctx context.Context) (bool, error) {
		if a.db == nil {
			return false, errors.NewInternalError("governance db is nil")
		}
		norm := rules.NormalizeResourceName(dnsName)
		var count int
		if err := a.db.QueryRowContext(ctx, queries.CountActiveDNSCooldownSQL, norm).Scan(&count); err != nil {
			return false, err
		}
		return count > 0, nil
	}, attribute.String("db.system", "postgresql"), attribute.String("dns.name", dnsName))
}

func (a *PostgresProvisioningGovernanceAdapter) AcquireSagaLock(ctx context.Context, serviceKey string, tenantID string, operation string) error {
	return observability.WithClientSpan(ctx, "self_service.governance.postgres.acquire_saga_lock", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("governance db is nil")
		}
		res, err := a.db.ExecContext(ctx, queries.AcquireSagaLockSQL, serviceKey, tenantID, operation)
		if err != nil {
			return err
		}
		if rows, _ := res.RowsAffected(); rows == 0 {
			return errors.NewConflictError(fmt.Sprintf("EC-13: service %s already has an active saga lock", serviceKey))
		}
		return nil
	}, attribute.String("db.system", "postgresql"), attribute.String("service.key", serviceKey))
}

func (a *PostgresProvisioningGovernanceAdapter) ReleaseSagaLock(ctx context.Context, serviceKey string, finalState string) error {
	return observability.WithClientSpan(ctx, "self_service.governance.postgres.release_saga_lock", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("governance db is nil")
		}
		_, err := a.db.ExecContext(ctx, queries.ReleaseSagaLockSQL, serviceKey, finalState)
		return err
	}, attribute.String("db.system", "postgresql"), attribute.String("service.key", serviceKey))
}

func (a *PostgresProvisioningGovernanceAdapter) RunSweeper(ctx context.Context) (*contracts.SweeperReport, error) {
	return observability.RunTracedClient(ctx, "self_service.governance.postgres.sweeper", func(ctx context.Context) (*contracts.SweeperReport, error) {
		if a.db == nil {
			return nil, errors.NewInternalError("governance db is nil")
		}
		report := &contracts.SweeperReport{
			UntrackedIntents: make([]string, 0),
			DuplicateTargets: make([]string, 0),
		}
		rows, err := a.db.QueryContext(ctx, queries.SweepUntrackedIntentsSQL)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				report.UntrackedIntents = append(report.UntrackedIntents, id)
			}
		}
		_ = rows.Close()

		dupRows, err := a.db.QueryContext(ctx, queries.SweepDuplicateReadyTargetsSQL)
		if err != nil {
			return nil, err
		}
		for dupRows.Next() {
			var target string
			if err := dupRows.Scan(&target); err == nil {
				report.DuplicateTargets = append(report.DuplicateTargets, target)
			}
		}
		_ = dupRows.Close()
		return report, nil
	}, attribute.String("db.system", "postgresql"))
}
