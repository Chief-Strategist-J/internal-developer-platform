package adapters

import (
	"context"
	"database/sql"
	"time"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/queries"
	"internal-developer-platform/src/shared/errors"
)

type PostgresWorkloadRepositoryAdapter struct {
	db *sql.DB
}

func NewPostgresWorkloadRepositoryAdapter(db *sql.DB) *PostgresWorkloadRepositoryAdapter {
	return &PostgresWorkloadRepositoryAdapter{db: db}
}

func (a *PostgresWorkloadRepositoryAdapter) Save(ctx context.Context, record contracts.WorkloadProvisionRecord) error {
	_, err := a.db.ExecContext(
		ctx,
		queries.FLOW_INSERT_WORKLOAD_PROVISION,
		record.ID,
		record.TenantID,
		record.ServiceID,
		record.Namespace,
		record.WorkloadName,
		record.CPURequest,
		record.CPULimit,
		record.MemoryRequest,
		record.MemoryLimit,
		record.Replicas,
		string(record.Status),
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

func (a *PostgresWorkloadRepositoryAdapter) GetByID(ctx context.Context, tenantID string, id string) (*contracts.WorkloadProvisionRecord, error) {
	row := a.db.QueryRowContext(ctx, queries.FLOW_SELECT_WORKLOAD_BY_ID, tenantID, id)
	var rec contracts.WorkloadProvisionRecord
	var status string
	err := row.Scan(
		&rec.ID,
		&rec.TenantID,
		&rec.ServiceID,
		&rec.Namespace,
		&rec.WorkloadName,
		&rec.CPURequest,
		&rec.CPULimit,
		&rec.MemoryRequest,
		&rec.MemoryLimit,
		&rec.Replicas,
		&status,
		&rec.IdempotencyKey,
		&rec.Version,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NewNotFoundError("workload provision record not found")
		}
		return nil, err
	}
	rec.Status = contracts.WorkloadStatus(status)
	return &rec, nil
}

func (a *PostgresWorkloadRepositoryAdapter) GetByIdempotencyKey(ctx context.Context, tenantID string, key string) (*contracts.WorkloadProvisionRecord, error) {
	row := a.db.QueryRowContext(ctx, queries.FLOW_SELECT_WORKLOAD_BY_IDEMPOTENCY, tenantID, key)
	var rec contracts.WorkloadProvisionRecord
	var status string
	err := row.Scan(
		&rec.ID,
		&rec.TenantID,
		&rec.ServiceID,
		&rec.Namespace,
		&rec.WorkloadName,
		&rec.CPURequest,
		&rec.CPULimit,
		&rec.MemoryRequest,
		&rec.MemoryLimit,
		&rec.Replicas,
		&status,
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
	rec.Status = contracts.WorkloadStatus(status)
	return &rec, nil
}

func (a *PostgresWorkloadRepositoryAdapter) List(ctx context.Context, filter contracts.FilterCriteria) ([]contracts.WorkloadProvisionRecord, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := a.db.QueryContext(ctx, queries.FLOW_LIST_WORKLOAD_PROVISIONS, filter.TenantID, limit, filter.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]contracts.WorkloadProvisionRecord, 0)
	for rows.Next() {
		var rec contracts.WorkloadProvisionRecord
		var status string
		if err := rows.Scan(
			&rec.ID,
			&rec.TenantID,
			&rec.ServiceID,
			&rec.Namespace,
			&rec.WorkloadName,
			&rec.CPURequest,
			&rec.CPULimit,
			&rec.MemoryRequest,
			&rec.MemoryLimit,
			&rec.Replicas,
			&status,
			&rec.IdempotencyKey,
			&rec.Version,
			&rec.CreatedAt,
			&rec.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rec.Status = contracts.WorkloadStatus(status)
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (a *PostgresWorkloadRepositoryAdapter) Update(ctx context.Context, record contracts.WorkloadProvisionRecord) error {
	res, err := a.db.ExecContext(
		ctx,
		queries.FLOW_UPDATE_WORKLOAD_PROVISION,
		string(record.Status),
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

