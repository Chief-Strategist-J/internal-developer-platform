/*
ALGORITHM & ARCHITECTURE BLUEPRINT: PLATFORM REGIME POSTGRES REPOSITORY ADAPTER

1. Database Adapter:
  - Implements PlatformRegimeRepositoryPort against PostgreSQL with connection pool safety.
  - Enforces transactional supersede on new active policies.
  - Zero comments inside method bodies.
*/
package adapters

import (
	"context"
	"database/sql"
	"encoding/json"

	"internal-developer-platform/src/features/platform-regime/contracts"
	"internal-developer-platform/src/features/platform-regime/infrastructure/queries"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type PostgresPlatformRegimeRepositoryAdapter struct {
	db *sql.DB
}

func NewPostgresPlatformRegimeRepositoryAdapter(db *sql.DB) *PostgresPlatformRegimeRepositoryAdapter {
	return &PostgresPlatformRegimeRepositoryAdapter{db: db}
}

func (p *PostgresPlatformRegimeRepositoryAdapter) Save(ctx context.Context, record contracts.PlatformRegimePolicyRecord) error {
	return observability.WithSpan(ctx, "postgres.regime.save", func(ctx context.Context) error {
		regionsJSON, err := json.Marshal(record.DataResidencyRegions)
		if err != nil {
			return errors.NewInternalError("failed to serialize data residency regions", err)
		}

		tx, err := p.db.BeginTx(ctx, nil)
		if err != nil {
			return errors.NewInternalError("failed to begin transaction", err)
		}
		defer func() {
			_ = tx.Rollback()
		}()

		_, err = tx.ExecContext(
			ctx,
			queries.FlowInsertRegimePolicy,
			record.ID,
			record.PublicID,
			record.TenantID,
			record.Source,
			record.ExternalID,
			record.CorrelationID,
			record.Jurisdiction,
			record.AuditRetentionDays,
			regionsJSON,
			record.KMSKeyCustodyMode,
			record.RTOMinutesTier0,
			record.RPOMinutesTier0,
			record.RTOMinutesTier1,
			record.RPOMinutesTier1,
			record.ClusterMode,
			record.PrimaryRegionCount,
			record.StandbyRegionCount,
			record.TenantIsolationMode,
			record.Status,
			record.Version,
			record.CreatedAt,
			record.UpdatedAt,
		)
		if err != nil {
			return errors.NewConflictError("failed to insert platform regime record: " + err.Error())
		}

		if record.Status == contracts.RegimeStatusActive {
			_, err = tx.ExecContext(ctx, queries.FlowSupersedePreviousActive, record.TenantID, record.ID)
			if err != nil {
				return errors.NewInternalError("failed to supersede previous active regime", err)
			}
		}

		if err := tx.Commit(); err != nil {
			return errors.NewInternalError("failed to commit transaction", err)
		}

		return nil
	})
}

func (p *PostgresPlatformRegimeRepositoryAdapter) GetActive(ctx context.Context, tenantID string) (*contracts.PlatformRegimePolicyRecord, error) {
	return observability.RunTraced(ctx, "postgres.regime.get_active", func(ctx context.Context) (*contracts.PlatformRegimePolicyRecord, error) {
		row := p.db.QueryRowContext(ctx, queries.FlowSelectActiveRegime, tenantID)
		return scanRegimeRecord(row)
	})
}

func (p *PostgresPlatformRegimeRepositoryAdapter) GetByID(ctx context.Context, tenantID string, id string) (*contracts.PlatformRegimePolicyRecord, error) {
	return observability.RunTraced(ctx, "postgres.regime.get_by_id", func(ctx context.Context) (*contracts.PlatformRegimePolicyRecord, error) {
		row := p.db.QueryRowContext(ctx, queries.FlowSelectRegimeByID, tenantID, id)
		return scanRegimeRecord(row)
	})
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanRegimeRecord(row scannable) (*contracts.PlatformRegimePolicyRecord, error) {
	var rec contracts.PlatformRegimePolicyRecord
	var regionsRaw []byte

	err := row.Scan(
		&rec.ID,
		&rec.PublicID,
		&rec.TenantID,
		&rec.Source,
		&rec.ExternalID,
		&rec.CorrelationID,
		&rec.Jurisdiction,
		&rec.AuditRetentionDays,
		&regionsRaw,
		&rec.KMSKeyCustodyMode,
		&rec.RTOMinutesTier0,
		&rec.RPOMinutesTier0,
		&rec.RTOMinutesTier1,
		&rec.RPOMinutesTier1,
		&rec.ClusterMode,
		&rec.PrimaryRegionCount,
		&rec.StandbyRegionCount,
		&rec.TenantIsolationMode,
		&rec.Status,
		&rec.Version,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, errors.NewNotFoundError("platform regime policy not found")
	}
	if err != nil {
		return nil, errors.NewInternalError("failed to scan regime policy row", err)
	}

	if len(regionsRaw) > 0 {
		var regions []string
		if err := json.Unmarshal(regionsRaw, &regions); err == nil {
			rec.DataResidencyRegions = regions
		}
	}

	return &rec, nil
}
