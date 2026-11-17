package adapters

import (
	"context"
	"database/sql"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/queries"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type PostgresWorkloadRepositoryAdapter struct {
	db *sql.DB
}

func NewPostgresWorkloadRepositoryAdapter(db *sql.DB) *PostgresWorkloadRepositoryAdapter {
	return &PostgresWorkloadRepositoryAdapter{db: db}
}

func (a *PostgresWorkloadRepositoryAdapter) Save(ctx context.Context, record contracts.WorkloadProvisionRecord) error {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "postgres.workload.save",
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "INSERT"),
		attribute.String("tenant.id", record.TenantID),
		attribute.String("workload.id", record.ID),
	)
	defer span.End()

	if record.TenantID == "" || record.ID == "" {
		err := errors.NewValidationError("tenantId and id are required")
		observability.RecordError(span, err)
		return err
	}

	strategy := string(record.Strategy)
	if strategy == "" {
		strategy = string(contracts.StrategyCanary)
	}

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
		strategy,
		string(record.Status),
		record.IdempotencyKey,
		record.Version,
		record.CreatedAt,
		record.UpdatedAt,
	)
	if err != nil {
		conflictErr := errors.NewConflictError(err.Error())
		observability.RecordError(span, conflictErr)
		return conflictErr
	}
	return nil
}

func (a *PostgresWorkloadRepositoryAdapter) GetByID(ctx context.Context, tenantID string, id string) (*contracts.WorkloadProvisionRecord, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "postgres.workload.get_by_id",
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "SELECT"),
		attribute.String("tenant.id", tenantID),
		attribute.String("workload.id", id),
	)
	defer span.End()

	if tenantID == "" || id == "" {
		err := errors.NewValidationError("tenantId and id are required")
		observability.RecordError(span, err)
		return nil, err
	}

	row := a.db.QueryRowContext(ctx, queries.FLOW_SELECT_WORKLOAD_BY_ID, tenantID, id)
	var rec contracts.WorkloadProvisionRecord
	var strategy string
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
		&strategy,
		&status,
		&rec.IdempotencyKey,
		&rec.Version,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			notFoundErr := errors.NewNotFoundError("workload provision record not found")
			observability.RecordError(span, notFoundErr)
			return nil, notFoundErr
		}
		observability.RecordError(span, err)
		return nil, err
	}
	rec.Strategy = contracts.DeploymentStrategy(strategy)
	rec.Status = contracts.WorkloadStatus(status)
	return &rec, nil
}

func (a *PostgresWorkloadRepositoryAdapter) GetByIdempotencyKey(ctx context.Context, tenantID string, key string) (*contracts.WorkloadProvisionRecord, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "postgres.workload.get_by_idempotency",
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "SELECT"),
		attribute.String("tenant.id", tenantID),
		attribute.String("idempotency.key", key),
	)
	defer span.End()

	if tenantID == "" || key == "" {
		err := errors.NewValidationError("tenantId and key are required")
		observability.RecordError(span, err)
		return nil, err
	}

	row := a.db.QueryRowContext(ctx, queries.FLOW_SELECT_WORKLOAD_BY_IDEMPOTENCY, tenantID, key)
	var rec contracts.WorkloadProvisionRecord
	var strategy string
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
		&strategy,
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
		observability.RecordError(span, err)
		return nil, err
	}
	rec.Strategy = contracts.DeploymentStrategy(strategy)
	rec.Status = contracts.WorkloadStatus(status)
	return &rec, nil
}

func (a *PostgresWorkloadRepositoryAdapter) List(ctx context.Context, filter contracts.FilterCriteria) ([]contracts.WorkloadProvisionRecord, error) {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "postgres.workload.list",
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "SELECT"),
		attribute.String("tenant.id", filter.TenantID),
	)
	defer span.End()

	if filter.TenantID == "" {
		err := errors.NewValidationError("tenantId is required")
		observability.RecordError(span, err)
		return nil, err
	}

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := a.db.QueryContext(ctx, queries.FLOW_LIST_WORKLOAD_PROVISIONS, filter.TenantID, limit, offset)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	defer rows.Close()

	records := make([]contracts.WorkloadProvisionRecord, 0)
	for rows.Next() {
		var rec contracts.WorkloadProvisionRecord
		var strategy string
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
			&strategy,
			&status,
			&rec.IdempotencyKey,
			&rec.Version,
			&rec.CreatedAt,
			&rec.UpdatedAt,
		); err != nil {
			observability.RecordError(span, err)
			return nil, err
		}
		rec.Strategy = contracts.DeploymentStrategy(strategy)
		rec.Status = contracts.WorkloadStatus(status)
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	return records, nil
}

func (a *PostgresWorkloadRepositoryAdapter) Update(ctx context.Context, record contracts.WorkloadProvisionRecord) error {
	tracer := observability.GetTracer()
	ctx, span := tracer.StartSpan(ctx, "postgres.workload.update",
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "UPDATE"),
		attribute.String("tenant.id", record.TenantID),
		attribute.String("workload.id", record.ID),
	)
	defer span.End()

	if record.TenantID == "" || record.ID == "" {
		err := errors.NewValidationError("tenantId and id are required")
		observability.RecordError(span, err)
		return err
	}

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
		observability.RecordError(span, err)
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		observability.RecordError(span, err)
		return err
	}
	if affected == 0 {
		conflictErr := errors.NewConflictError("optimistic concurrency conflict on update")
		observability.RecordError(span, conflictErr)
		return conflictErr
	}
	return nil
}
