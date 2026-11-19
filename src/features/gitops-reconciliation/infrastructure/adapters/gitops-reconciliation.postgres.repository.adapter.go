package adapters

import (
	"context"
	"database/sql"
	"time"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/infrastructure/queries"
	"internal-developer-platform/src/shared/errors"
)

type PostgresGitOpsRepositoryAdapter struct {
	db *sql.DB
}

func NewPostgresGitOpsRepositoryAdapter(db *sql.DB) *PostgresGitOpsRepositoryAdapter {
	return &PostgresGitOpsRepositoryAdapter{db: db}
}

func (a *PostgresGitOpsRepositoryAdapter) Save(ctx context.Context, record contracts.ReconciliationRecord) error {
	_, err := a.db.ExecContext(
		ctx,
		queries.FLOW_INSERT_RECONCILIATION,
		record.ID,
		record.TenantID,
		record.ServiceID,
		record.AppName,
		record.TargetRevision,
		record.CurrentRevision,
		string(record.SyncPhase),
		string(record.HealthStatus),
		record.IdempotencyKey,
		record.Version,
		record.CreatedAt,
		record.UpdatedAt,
	)
	if err != nil {
		return errors.NewConflictError(err.Error())
	}
	return nil
}

func (a *PostgresGitOpsRepositoryAdapter) GetByID(ctx context.Context, tenantID string, id string) (*contracts.ReconciliationRecord, error) {
	row := a.db.QueryRowContext(ctx, queries.FLOW_SELECT_RECONCILIATION_BY_ID, tenantID, id)
	var rec contracts.ReconciliationRecord
	var phase string
	var health string
	err := row.Scan(
		&rec.ID,
		&rec.TenantID,
		&rec.ServiceID,
		&rec.AppName,
		&rec.TargetRevision,
		&rec.CurrentRevision,
		&phase,
		&health,
		&rec.IdempotencyKey,
		&rec.Version,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NewNotFoundError("reconciliation record not found")
		}
		return nil, err
	}
	rec.SyncPhase = contracts.SyncPhase(phase)
	rec.HealthStatus = contracts.HealthStatus(health)
	return &rec, nil
}

func (a *PostgresGitOpsRepositoryAdapter) GetByIdempotencyKey(ctx context.Context, tenantID string, key string) (*contracts.ReconciliationRecord, error) {
	row := a.db.QueryRowContext(ctx, queries.FLOW_SELECT_RECONCILIATION_BY_IDEMPOTENCY, tenantID, key)
	var rec contracts.ReconciliationRecord
	var phase string
	var health string
	err := row.Scan(
		&rec.ID,
		&rec.TenantID,
		&rec.ServiceID,
		&rec.AppName,
		&rec.TargetRevision,
		&rec.CurrentRevision,
		&phase,
		&health,
		&rec.IdempotencyKey,
		&rec.Version,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	rec.SyncPhase = contracts.SyncPhase(phase)
	rec.HealthStatus = contracts.HealthStatus(health)
	return &rec, nil
}

func (a *PostgresGitOpsRepositoryAdapter) GetLatestSyncedRecord(ctx context.Context, tenantID string, serviceID string) (*contracts.ReconciliationRecord, error) {
	row := a.db.QueryRowContext(ctx, queries.FLOW_SELECT_LATEST_SYNCED, tenantID, serviceID)
	var rec contracts.ReconciliationRecord
	var phase string
	var health string
	err := row.Scan(
		&rec.ID,
		&rec.TenantID,
		&rec.ServiceID,
		&rec.AppName,
		&rec.TargetRevision,
		&rec.CurrentRevision,
		&phase,
		&health,
		&rec.IdempotencyKey,
		&rec.Version,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	rec.SyncPhase = contracts.SyncPhase(phase)
	rec.HealthStatus = contracts.HealthStatus(health)
	return &rec, nil
}

func (a *PostgresGitOpsRepositoryAdapter) List(ctx context.Context, filter contracts.FilterCriteria) ([]contracts.ReconciliationRecord, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := a.db.QueryContext(ctx, queries.FLOW_LIST_RECONCILIATIONS, filter.TenantID, limit, filter.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]contracts.ReconciliationRecord, 0)
	for rows.Next() {
		var rec contracts.ReconciliationRecord
		var phase string
		var health string
		if err := rows.Scan(
			&rec.ID,
			&rec.TenantID,
			&rec.ServiceID,
			&rec.AppName,
			&rec.TargetRevision,
			&rec.CurrentRevision,
			&phase,
			&health,
			&rec.IdempotencyKey,
			&rec.Version,
			&rec.CreatedAt,
			&rec.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rec.SyncPhase = contracts.SyncPhase(phase)
		rec.HealthStatus = contracts.HealthStatus(health)
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (a *PostgresGitOpsRepositoryAdapter) Update(ctx context.Context, record contracts.ReconciliationRecord) error {
	res, err := a.db.ExecContext(
		ctx,
		queries.FLOW_UPDATE_RECONCILIATION,
		record.CurrentRevision,
		string(record.SyncPhase),
		string(record.HealthStatus),
		time.Now().UTC(),
		record.TenantID,
		record.ID,
		record.Version,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.NewConflictError("optimistic concurrency conflict on update")
	}
	return nil
}
