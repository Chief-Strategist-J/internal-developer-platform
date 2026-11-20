/*
ALGORITHM & ARCHITECTURE BLUEPRINT: POSTGRESQL WORKLOAD REPOSITORY ADAPTER

1. Persistence Flow Execution:
   - Implements WorkloadStorePort backed by PostgreSQL 18.
   - Enforces the 5-identifier roles contract: id (surrogate), public_id (external), tenant_id (scope),
     source + external_id (natural key), and correlation_id (causation trace).
   - Validates entity boundary prerequisites before issuing parameterized SQL statements.
   - Maps database rows to strongly typed WorkloadProvisionRecord domain entities.
   - Integrates W3C distributed tracing with standard OpenTelemetry db.system attributes.

2. Invariants:
   - Zero inline comments inside method bodies.
   - All methods strictly maintain a length <= 25 lines.
   - Zero raw SQL queries; all statements reference queries.FLOW_* constants.
*/
package adapters

import (
	"context"
	"database/sql"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/contracts"
	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/queries"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type PostgresWorkloadRepositoryAdapter struct {
	db *sql.DB
}

type rowScanner interface {
	Scan(dest ...any) error
}

func NewPostgresWorkloadRepositoryAdapter(db *sql.DB) *PostgresWorkloadRepositoryAdapter {
	return &PostgresWorkloadRepositoryAdapter{db: db}
}

func (a *PostgresWorkloadRepositoryAdapter) Save(ctx context.Context, record contracts.WorkloadProvisionRecord) error {
	ctx, span := startWorkloadSpan(ctx, "postgres.workload.save", "INSERT", record.TenantID, record.ID)
	defer span.End()

	if record.TenantID == "" || record.ID == "" {
		err := errors.NewValidationError("tenantId and id are required")
		observability.RecordError(span, err)
		return err
	}

	args := resolveRecordFields(record)
	if _, err := a.db.ExecContext(ctx, queries.FLOW_INSERT_WORKLOAD_PROVISION, args...); err != nil {
		conflictErr := errors.NewConflictError(err.Error())
		observability.RecordError(span, conflictErr)
		return conflictErr
	}
	return nil
}

func (a *PostgresWorkloadRepositoryAdapter) GetByID(ctx context.Context, tenantID string, id string) (*contracts.WorkloadProvisionRecord, error) {
	ctx, span := startWorkloadSpan(ctx, "postgres.workload.get_by_id", "SELECT", tenantID, id)
	defer span.End()

	if tenantID == "" || id == "" {
		err := errors.NewValidationError("tenantId and id are required")
		observability.RecordError(span, err)
		return nil, err
	}

	row := a.db.QueryRowContext(ctx, queries.FLOW_SELECT_WORKLOAD_BY_ID, tenantID, id)
	rec, err := scanWorkloadRecord(row)
	if err != nil {
		if err == sql.ErrNoRows {
			notFoundErr := errors.NewNotFoundError("workload provision record not found")
			observability.RecordError(span, notFoundErr)
			return nil, notFoundErr
		}
		observability.RecordError(span, err)
		return nil, err
	}
	return rec, nil
}

func (a *PostgresWorkloadRepositoryAdapter) GetByIdempotencyKey(ctx context.Context, tenantID string, key string) (*contracts.WorkloadProvisionRecord, error) {
	ctx, span := startWorkloadSpan(ctx, "postgres.workload.get_by_idempotency", "SELECT", tenantID, "")
	defer span.End()

	if tenantID == "" || key == "" {
		err := errors.NewValidationError("tenantId and key are required")
		observability.RecordError(span, err)
		return nil, err
	}

	row := a.db.QueryRowContext(ctx, queries.FLOW_SELECT_WORKLOAD_BY_IDEMPOTENCY, tenantID, key)
	rec, err := scanWorkloadRecord(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		observability.RecordError(span, err)
		return nil, err
	}
	return rec, nil
}

func (a *PostgresWorkloadRepositoryAdapter) List(ctx context.Context, filter contracts.FilterCriteria) ([]contracts.WorkloadProvisionRecord, error) {
	ctx, span := startWorkloadSpan(ctx, "postgres.workload.list", "SELECT", filter.TenantID, "")
	defer span.End()

	if filter.TenantID == "" {
		err := errors.NewValidationError("tenantId is required")
		observability.RecordError(span, err)
		return nil, err
	}

	limit, offset := sanitizePagination(filter.Limit, filter.Offset)
	rows, err := a.db.QueryContext(ctx, queries.FLOW_LIST_WORKLOAD_PROVISIONS, filter.TenantID, limit, offset)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	defer rows.Close()

	return scanWorkloadRows(rows)
}

func (a *PostgresWorkloadRepositoryAdapter) Update(ctx context.Context, record contracts.WorkloadProvisionRecord) error {
	ctx, span := startWorkloadSpan(ctx, "postgres.workload.update", "UPDATE", record.TenantID, record.ID)
	defer span.End()

	if record.TenantID == "" || record.ID == "" {
		err := errors.NewValidationError("tenantId and id are required")
		observability.RecordError(span, err)
		return err
	}

	res, err := a.db.ExecContext(ctx, queries.FLOW_UPDATE_WORKLOAD_PROVISION,
		string(record.Status), record.UpdatedAt, record.TenantID, record.ID, record.Version,
	)
	if err != nil {
		observability.RecordError(span, err)
		return err
	}
	return verifyRowsAffected(res, span)
}

func scanWorkloadRecord(s rowScanner) (*contracts.WorkloadProvisionRecord, error) {
	var rec contracts.WorkloadProvisionRecord
	var strategy string
	var status string
	err := s.Scan(
		&rec.ID, &rec.PublicID, &rec.TenantID, &rec.Source, &rec.ExternalID, &rec.CorrelationID,
		&rec.SpecHash, &rec.ApprovedSpecHash, &rec.ApproverRole,
		&rec.ImagePullPolicy, &rec.BreakglassJustification,
		&rec.ServiceID, &rec.Namespace, &rec.WorkloadName, &rec.CPURequest, &rec.CPULimit,
		&rec.MemoryRequest, &rec.MemoryLimit, &rec.Replicas, &strategy, &status,
		&rec.IdempotencyKey, &rec.Version, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	rec.Strategy = contracts.DeploymentStrategy(strategy)
	rec.Status = contracts.WorkloadStatus(status)
	return &rec, nil
}

func resolveImagePullPolicy(policy string) string {
	if policy == "" {
		return "IfNotPresent"
	}
	return policy
}

func resolveRecordFields(record contracts.WorkloadProvisionRecord) []interface{} {
	return []interface{}{
		record.ID, resolvePublicID(record.PublicID, record.ID), record.TenantID,
		resolveSource(record.Source), resolveExternalID(record.ExternalID, record.ID),
		resolveCorrelationID(record.CorrelationID), record.SpecHash, record.ApprovedSpecHash,
		record.ApproverRole, resolveImagePullPolicy(record.ImagePullPolicy), record.BreakglassJustification,
		record.ServiceID, record.Namespace, record.WorkloadName, record.CPURequest, record.CPULimit,
		record.MemoryRequest, record.MemoryLimit, record.Replicas, resolveStrategy(record.Strategy),
		string(record.Status), record.IdempotencyKey, record.Version, record.CreatedAt, record.UpdatedAt,
	}
}

func scanWorkloadRows(rows *sql.Rows) ([]contracts.WorkloadProvisionRecord, error) {
	var records []contracts.WorkloadProvisionRecord
	for rows.Next() {
		rec, err := scanWorkloadRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}
	return records, rows.Err()
}

func startWorkloadSpan(ctx context.Context, name, op, tenantID, workloadID string) (context.Context, trace.Span) {
	tracer := observability.GetTracer()
	return tracer.StartSpan(ctx, name,
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", op),
		attribute.String("tenant.id", tenantID),
		attribute.String("workload.id", workloadID),
	)
}

func resolveStrategy(strategy contracts.DeploymentStrategy) string {
	if strategy == "" {
		return string(contracts.StrategyCanary)
	}
	return string(strategy)
}

func resolvePublicID(publicID, id string) string {
	if publicID != "" {
		return publicID
	}
	clean := strings.ReplaceAll(id, "-", "")
	if len(clean) > 26 {
		clean = clean[:26]
	}
	return "wld_" + clean
}

func resolveSource(source string) string {
	if source != "" {
		return source
	}
	return "api"
}

func resolveExternalID(externalID, id string) string {
	if externalID != "" {
		return externalID
	}
	return id
}

func resolveCorrelationID(correlationID string) string {
	if _, err := uuid.Parse(correlationID); err == nil {
		return correlationID
	}
	return uuid.New().String()
}

func sanitizePagination(limit, offset int) (int, int) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func verifyRowsAffected(res sql.Result, span trace.Span) error {
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		observability.RecordError(span, err)
		return err
	}
	if rowsAffected == 0 {
		conflictErr := errors.NewConflictError("optimistic lock failed: version mismatch or record not found")
		observability.RecordError(span, conflictErr)
		return conflictErr
	}
	return nil
}
