package adapters

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/policy-as-code/contracts"
	"internal-developer-platform/src/features/policy-as-code/infrastructure/ports"
	"internal-developer-platform/src/features/policy-as-code/infrastructure/queries"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

const (
	SpanSavePolicy                     = "db.policy.save"
	SpanGetPolicyByID                  = "db.policy.get_by_id"
	SpanGetPolicyByPublicID            = "db.policy.get_by_public_id"
	SpanListPoliciesKeyset             = "db.policy.list_keyset"
	SpanListActiveCELPolicies          = "db.policy.list_active_cel"
	SpanUpdatePolicy                   = "db.policy.update"
	SpanUpdatePolicyLifecycle          = "db.policy.update_lifecycle"
	SpanSoftDeletePolicy               = "db.policy.soft_delete"
	SpanSaveException                  = "db.policy_exception.save"
	SpanGetExceptionByID               = "db.policy_exception.get_by_id"
	SpanGetActiveExceptionsForWorkload = "db.policy_exception.get_active_for_workload"
	SpanUpdateExceptionStatus          = "db.policy_exception.update_status"
	SpanReconcileExpiredExceptions     = "db.policy_exception.reconcile_expired"
	SpanSaveViolation                  = "db.policy_violation.save"
	SpanListActiveViolationsByPolicy   = "db.policy_violation.list_active_by_policy"
	SpanCountActiveViolationsByPolicy  = "db.policy_violation.count_active_by_policy"
	SpanResolveViolation               = "db.policy_violation.resolve"

	AttrTenantID         = "db.tenant_id"
	AttrPolicyID         = "db.policy_id"
	AttrPublicID         = "db.public_id"
	AttrWorkloadID       = "db.workload_id"
	AttrNamespace        = "db.namespace"
	AttrViolationID      = "db.violation_id"
	AttrSeverity         = "db.severity"
	AttrEnforcementPoint = "db.enforcement_point"
	AttrSource           = "db.source"

	ErrPolicyNotFoundMsg         = "policy not found"
	ErrExceptionNotFoundMsg      = "policy exception not found"
	ErrPolicyConflictMsg         = "policy update conflict: stale version or entity deleted"
	ErrExceptionConflictMsg      = "policy exception update conflict: stale version or entity deleted"
	ErrIllegalStateTransitionMsg = "illegal policy lifecycle state transition"
	ErrEmptyTenantIDMsg          = "tenant id must not be empty"
	ErrEmptyIDMsg                = "id must not be empty"
	ErrEmptyNamespaceMsg         = "namespace must not be empty"
	ErrEmptyWorkloadMsg          = "workload id must not be empty"
)

type PostgresPolicyRepositoryAdapter struct {
	db *sql.DB
}

var _ ports.PolicyRepositoryPort = (*PostgresPolicyRepositoryAdapter)(nil)

func NewPostgresPolicyRepositoryAdapter(db *sql.DB) *PostgresPolicyRepositoryAdapter {
	return &PostgresPolicyRepositoryAdapter{db: db}
}

func (a *PostgresPolicyRepositoryAdapter) SavePolicy(ctx context.Context, policy contracts.PolicyRecord) error {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanSavePolicy,
		attribute.String(AttrTenantID, policy.TenantID),
		attribute.String(AttrPolicyID, policy.ID),
	)
	defer span.End()

	if err := policy.Validate(); err != nil {
		observability.RecordError(span, err)
		return err
	}

	_, err := a.db.ExecContext(
		ctx,
		queries.FLOW_INSERT_POLICY,
		policy.ID,
		policy.PublicID,
		policy.TenantID,
		string(policy.Source),
		policy.ExternalID,
		policy.CorrelationID,
		policy.Name,
		policy.Description,
		string(policy.EnforcementPoint),
		string(policy.Severity),
		string(policy.LifecycleState),
		policy.RuleExpression,
		policy.Exceptable,
		policy.DetectionPair,
		policy.Version,
		policy.CreatedAt,
		policy.UpdatedAt,
	)
	if err != nil {
		observability.RecordError(span, err)
		return errors.NewConflictError(err.Error())
	}
	return nil
}

func (a *PostgresPolicyRepositoryAdapter) GetPolicyByID(ctx context.Context, tenantID string, id string) (*contracts.PolicyRecord, error) {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanGetPolicyByID,
		attribute.String(AttrTenantID, tenantID),
		attribute.String(AttrPolicyID, id),
	)
	defer span.End()

	if strings.TrimSpace(tenantID) == "" {
		err := errors.NewValidationError(ErrEmptyTenantIDMsg)
		observability.RecordError(span, err)
		return nil, err
	}
	if strings.TrimSpace(id) == "" {
		err := errors.NewValidationError(ErrEmptyIDMsg)
		observability.RecordError(span, err)
		return nil, err
	}

	row := a.db.QueryRowContext(ctx, queries.FLOW_SELECT_POLICY_BY_ID, tenantID, id)
	record, err := scanPolicyRecord(row)
	if err != nil {
		if err == sql.ErrNoRows {
			notErr := errors.NewNotFoundError(ErrPolicyNotFoundMsg)
			observability.RecordError(span, notErr)
			return nil, notErr
		}
		observability.RecordError(span, err)
		return nil, err
	}
	return record, nil
}

func (a *PostgresPolicyRepositoryAdapter) GetPolicyByPublicID(ctx context.Context, tenantID string, publicID string) (*contracts.PolicyRecord, error) {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanGetPolicyByPublicID,
		attribute.String(AttrTenantID, tenantID),
		attribute.String(AttrPublicID, publicID),
	)
	defer span.End()

	if strings.TrimSpace(tenantID) == "" {
		err := errors.NewValidationError(ErrEmptyTenantIDMsg)
		observability.RecordError(span, err)
		return nil, err
	}
	if strings.TrimSpace(publicID) == "" {
		err := errors.NewValidationError(ErrEmptyIDMsg)
		observability.RecordError(span, err)
		return nil, err
	}

	row := a.db.QueryRowContext(ctx, queries.FLOW_SELECT_POLICY_BY_PUBLIC_ID, tenantID, publicID)
	record, err := scanPolicyRecord(row)
	if err != nil {
		if err == sql.ErrNoRows {
			notErr := errors.NewNotFoundError(ErrPolicyNotFoundMsg)
			observability.RecordError(span, notErr)
			return nil, notErr
		}
		observability.RecordError(span, err)
		return nil, err
	}
	return record, nil
}

func (a *PostgresPolicyRepositoryAdapter) ListPoliciesKeyset(ctx context.Context, filter contracts.KeysetFilterCriteria) ([]contracts.PolicyRecord, error) {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanListPoliciesKeyset,
		attribute.String(AttrTenantID, filter.TenantID),
	)
	defer span.End()

	if strings.TrimSpace(filter.TenantID) == "" {
		err := errors.NewValidationError(ErrEmptyTenantIDMsg)
		observability.RecordError(span, err)
		return nil, err
	}

	limit := filter.PageSize
	if limit <= 0 {
		limit = contracts.DefaultPageSize
	}
	if limit > contracts.MaxPageSize {
		limit = contracts.MaxPageSize
	}

	rows, err := a.db.QueryContext(ctx, queries.FLOW_LIST_POLICIES_KEYSET, filter.TenantID, filter.CursorID, limit)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	defer rows.Close()

	var records []contracts.PolicyRecord
	for rows.Next() {
		record, err := scanPolicyRecord(rows)
		if err != nil {
			observability.RecordError(span, err)
			return nil, err
		}
		records = append(records, *record)
	}
	if err := rows.Err(); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	return records, nil
}

func (a *PostgresPolicyRepositoryAdapter) ListActiveCELPolicies(ctx context.Context, tenantID string) ([]contracts.PolicyRecord, error) {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanListActiveCELPolicies,
		attribute.String(AttrTenantID, tenantID),
	)
	defer span.End()

	if strings.TrimSpace(tenantID) == "" {
		err := errors.NewValidationError(ErrEmptyTenantIDMsg)
		observability.RecordError(span, err)
		return nil, err
	}

	rows, err := a.db.QueryContext(ctx, queries.FLOW_LIST_ACTIVE_CEL_POLICIES, tenantID)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	defer rows.Close()

	var records []contracts.PolicyRecord
	for rows.Next() {
		record, err := scanPolicyRecord(rows)
		if err != nil {
			observability.RecordError(span, err)
			return nil, err
		}
		records = append(records, *record)
	}
	if err := rows.Err(); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	return records, nil
}

func (a *PostgresPolicyRepositoryAdapter) UpdatePolicy(ctx context.Context, policy contracts.PolicyRecord, expectedVersion int64) error {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanUpdatePolicy,
		attribute.String(AttrTenantID, policy.TenantID),
		attribute.String(AttrPolicyID, policy.ID),
	)
	defer span.End()

	if err := policy.Validate(); err != nil {
		observability.RecordError(span, err)
		return err
	}

	res, err := a.db.ExecContext(
		ctx,
		queries.FLOW_UPDATE_POLICY,
		policy.Name,
		policy.Description,
		policy.RuleExpression,
		policy.Exceptable,
		policy.DetectionPair,
		time.Now().UTC(),
		policy.TenantID,
		policy.ID,
		expectedVersion,
	)
	if err != nil {
		observability.RecordError(span, err)
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		observability.RecordError(span, err)
		return err
	}
	if rowsAffected == 0 {
		conflictErr := errors.NewConflictError(ErrPolicyConflictMsg)
		observability.RecordError(span, conflictErr)
		return conflictErr
	}
	return nil
}

func (a *PostgresPolicyRepositoryAdapter) UpdatePolicyLifecycle(ctx context.Context, tenantID string, id string, newState contracts.PolicyLifecycleState, expectedVersion int64) error {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanUpdatePolicyLifecycle,
		attribute.String(AttrTenantID, tenantID),
		attribute.String(AttrPolicyID, id),
	)
	defer span.End()

	existing, err := a.GetPolicyByID(ctx, tenantID, id)
	if err != nil {
		observability.RecordError(span, err)
		return err
	}

	var valid int
	err = a.db.QueryRowContext(ctx, queries.FLOW_VERIFY_STATE_TRANSITION, string(existing.LifecycleState), string(newState)).Scan(&valid)
	if err != nil {
		transErr := errors.NewValidationError(ErrIllegalStateTransitionMsg)
		observability.RecordError(span, transErr)
		return transErr
	}

	res, err := a.db.ExecContext(
		ctx,
		queries.FLOW_UPDATE_POLICY_LIFECYCLE,
		string(newState),
		time.Now().UTC(),
		tenantID,
		id,
		expectedVersion,
	)
	if err != nil {
		observability.RecordError(span, err)
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		observability.RecordError(span, err)
		return err
	}
	if rowsAffected == 0 {
		conflictErr := errors.NewConflictError(ErrPolicyConflictMsg)
		observability.RecordError(span, conflictErr)
		return conflictErr
	}
	return nil
}

func (a *PostgresPolicyRepositoryAdapter) SoftDeletePolicy(ctx context.Context, tenantID string, id string, expectedVersion int64) error {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanSoftDeletePolicy,
		attribute.String(AttrTenantID, tenantID),
		attribute.String(AttrPolicyID, id),
	)
	defer span.End()

	now := time.Now().UTC()
	res, err := a.db.ExecContext(ctx, queries.FLOW_SOFT_DELETE_POLICY, now, tenantID, id, expectedVersion)
	if err != nil {
		observability.RecordError(span, err)
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		observability.RecordError(span, err)
		return err
	}
	if rowsAffected == 0 {
		conflictErr := errors.NewConflictError(ErrPolicyConflictMsg)
		observability.RecordError(span, conflictErr)
		return conflictErr
	}
	return nil
}

func (a *PostgresPolicyRepositoryAdapter) SaveException(ctx context.Context, exception contracts.PolicyExceptionRecord) error {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanSaveException,
		attribute.String(AttrTenantID, exception.TenantID),
		attribute.String(AttrPolicyID, exception.PolicyID),
	)
	defer span.End()

	if err := exception.Validate(); err != nil {
		observability.RecordError(span, err)
		return err
	}

	_, err := a.db.ExecContext(
		ctx,
		queries.FLOW_INSERT_EXCEPTION,
		exception.ID,
		exception.PublicID,
		exception.TenantID,
		exception.PolicyID,
		exception.WorkloadID,
		exception.Namespace,
		exception.ApproverID,
		exception.Justification,
		string(exception.Status),
		exception.ExpiresAt,
		exception.CorrelationID,
		exception.Version,
		exception.CreatedAt,
		exception.UpdatedAt,
	)
	if err != nil {
		observability.RecordError(span, err)
		return errors.NewConflictError(err.Error())
	}
	return nil
}

func (a *PostgresPolicyRepositoryAdapter) GetExceptionByID(ctx context.Context, tenantID string, id string) (*contracts.PolicyExceptionRecord, error) {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanGetExceptionByID,
		attribute.String(AttrTenantID, tenantID),
		attribute.String(AttrPolicyID, id),
	)
	defer span.End()

	if strings.TrimSpace(tenantID) == "" {
		err := errors.NewValidationError(ErrEmptyTenantIDMsg)
		observability.RecordError(span, err)
		return nil, err
	}
	if strings.TrimSpace(id) == "" {
		err := errors.NewValidationError(ErrEmptyIDMsg)
		observability.RecordError(span, err)
		return nil, err
	}

	row := a.db.QueryRowContext(ctx, queries.FLOW_SELECT_EXCEPTION_BY_ID, tenantID, id)
	record, err := scanExceptionRecord(row)
	if err != nil {
		if err == sql.ErrNoRows {
			notErr := errors.NewNotFoundError(ErrExceptionNotFoundMsg)
			observability.RecordError(span, notErr)
			return nil, notErr
		}
		observability.RecordError(span, err)
		return nil, err
	}
	return record, nil
}

func (a *PostgresPolicyRepositoryAdapter) GetActiveExceptionsForWorkload(ctx context.Context, tenantID string, namespace string, workloadID string) ([]contracts.PolicyExceptionRecord, error) {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanGetActiveExceptionsForWorkload,
		attribute.String(AttrTenantID, tenantID),
		attribute.String(AttrNamespace, namespace),
		attribute.String(AttrWorkloadID, workloadID),
	)
	defer span.End()

	if strings.TrimSpace(tenantID) == "" {
		err := errors.NewValidationError(ErrEmptyTenantIDMsg)
		observability.RecordError(span, err)
		return nil, err
	}
	if strings.TrimSpace(namespace) == "" {
		err := errors.NewValidationError(ErrEmptyNamespaceMsg)
		observability.RecordError(span, err)
		return nil, err
	}
	if strings.TrimSpace(workloadID) == "" {
		err := errors.NewValidationError(ErrEmptyWorkloadMsg)
		observability.RecordError(span, err)
		return nil, err
	}

	rows, err := a.db.QueryContext(ctx, queries.FLOW_SELECT_ACTIVE_EXCEPTIONS_FOR_WORKLOAD, tenantID, namespace, workloadID, time.Now().UTC())
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	defer rows.Close()

	var records []contracts.PolicyExceptionRecord
	for rows.Next() {
		record, err := scanExceptionRecord(rows)
		if err != nil {
			observability.RecordError(span, err)
			return nil, err
		}
		records = append(records, *record)
	}
	if err := rows.Err(); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	return records, nil
}

func (a *PostgresPolicyRepositoryAdapter) UpdateExceptionStatus(ctx context.Context, tenantID string, id string, newStatus contracts.ExceptionStatus, expectedVersion int64) error {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanUpdateExceptionStatus,
		attribute.String(AttrTenantID, tenantID),
		attribute.String(AttrPolicyID, id),
	)
	defer span.End()

	if strings.TrimSpace(tenantID) == "" {
		err := errors.NewValidationError(ErrEmptyTenantIDMsg)
		observability.RecordError(span, err)
		return err
	}
	if strings.TrimSpace(id) == "" {
		err := errors.NewValidationError(ErrEmptyIDMsg)
		observability.RecordError(span, err)
		return err
	}

	res, err := a.db.ExecContext(ctx, queries.FLOW_UPDATE_EXCEPTION_STATUS, string(newStatus), time.Now().UTC(), tenantID, id, expectedVersion)
	if err != nil {
		observability.RecordError(span, err)
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		observability.RecordError(span, err)
		return err
	}
	if rowsAffected == 0 {
		conflictErr := errors.NewConflictError(ErrExceptionConflictMsg)
		observability.RecordError(span, conflictErr)
		return conflictErr
	}
	return nil
}

func (a *PostgresPolicyRepositoryAdapter) ReconcileExpiredExceptions(ctx context.Context, tenantID string, now time.Time) (int, error) {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanReconcileExpiredExceptions,
		attribute.String(AttrTenantID, tenantID),
	)
	defer span.End()

	if strings.TrimSpace(tenantID) == "" {
		err := errors.NewValidationError(ErrEmptyTenantIDMsg)
		observability.RecordError(span, err)
		return 0, err
	}

	res, err := a.db.ExecContext(ctx, queries.FLOW_RECONCILE_EXPIRED_EXCEPTIONS, now, tenantID, now)
	if err != nil {
		observability.RecordError(span, err)
		return 0, err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		observability.RecordError(span, err)
		return 0, err
	}

	return int(rowsAffected), nil
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanPolicyRecord(scanner rowScanner) (*contracts.PolicyRecord, error) {
	var (
		rec         contracts.PolicyRecord
		source      string
		enforcement string
		severity    string
		lifecycle   string
	)

	err := scanner.Scan(
		&rec.ID,
		&rec.PublicID,
		&rec.TenantID,
		&source,
		&rec.ExternalID,
		&rec.CorrelationID,
		&rec.Name,
		&rec.Description,
		&enforcement,
		&severity,
		&lifecycle,
		&rec.RuleExpression,
		&rec.Exceptable,
		&rec.DetectionPair,
		&rec.Version,
		&rec.CreatedAt,
		&rec.UpdatedAt,
		&rec.DeletedAt,
	)
	if err != nil {
		return nil, err
	}

	rec.Source = contracts.PolicySource(source)
	rec.EnforcementPoint = contracts.EnforcementPoint(enforcement)
	rec.Severity = contracts.PolicySeverity(severity)
	rec.LifecycleState = contracts.PolicyLifecycleState(lifecycle)
	return &rec, nil
}

func scanExceptionRecord(scanner rowScanner) (*contracts.PolicyExceptionRecord, error) {
	var (
		rec    contracts.PolicyExceptionRecord
		status string
	)

	err := scanner.Scan(
		&rec.ID,
		&rec.PublicID,
		&rec.TenantID,
		&rec.PolicyID,
		&rec.WorkloadID,
		&rec.Namespace,
		&rec.ApproverID,
		&rec.Justification,
		&status,
		&rec.ExpiresAt,
		&rec.CorrelationID,
		&rec.Version,
		&rec.CreatedAt,
		&rec.UpdatedAt,
		&rec.DeletedAt,
	)
	if err != nil {
		return nil, err
	}

	rec.Status = contracts.ExceptionStatus(status)
	return &rec, nil
}

func (a *PostgresPolicyRepositoryAdapter) SaveViolation(ctx context.Context, v contracts.PolicyViolationRecord) error {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanSaveViolation,
		attribute.String(AttrTenantID, v.TenantID),
		attribute.String(AttrPolicyID, v.PolicyID),
		attribute.String(AttrPublicID, v.PublicID),
		attribute.String(AttrWorkloadID, v.WorkloadID),
		attribute.String(AttrNamespace, v.Namespace),
		attribute.String(AttrSeverity, string(v.Severity)),
		attribute.String(AttrEnforcementPoint, string(v.EnforcementPoint)),
		attribute.String(AttrSource, v.Source),
	)
	defer span.End()

	if err := v.Validate(); err != nil {
		observability.RecordError(span, err)
		return err
	}

	_, err := a.db.ExecContext(
		ctx,
		queries.FLOW_INSERT_VIOLATION,
		v.ID,
		v.PublicID,
		v.TenantID,
		v.PolicyID,
		v.Source,
		v.ExternalID,
		v.CorrelationID,
		v.WorkloadID,
		v.Namespace,
		string(v.Severity),
		string(v.EnforcementPoint),
		string(v.Status),
		v.Message,
		v.DetectionPair,
		v.Version,
		v.CreatedAt,
		v.UpdatedAt,
	)
	if err != nil {
		observability.RecordError(span, err)
		return errors.NewConflictError(err.Error())
	}
	return nil
}

func (a *PostgresPolicyRepositoryAdapter) ListActiveViolationsByPolicy(ctx context.Context, tenantID string, policyID string) ([]contracts.PolicyViolationRecord, error) {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanListActiveViolationsByPolicy,
		attribute.String(AttrTenantID, tenantID),
		attribute.String(AttrPolicyID, policyID),
	)
	defer span.End()

	rows, err := a.db.QueryContext(ctx, queries.FLOW_SELECT_ACTIVE_VIOLATIONS_BY_POLICY, tenantID, policyID)
	if err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	defer rows.Close()

	var result []contracts.PolicyViolationRecord
	for rows.Next() {
		rec, err := scanViolationRecord(rows)
		if err != nil {
			observability.RecordError(span, err)
			return nil, err
		}
		result = append(result, *rec)
	}

	if err := rows.Err(); err != nil {
		observability.RecordError(span, err)
		return nil, err
	}
	return result, nil
}

func (a *PostgresPolicyRepositoryAdapter) CountActiveViolationsByPolicy(ctx context.Context, tenantID string, policyID string) (int, error) {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanCountActiveViolationsByPolicy,
		attribute.String(AttrTenantID, tenantID),
		attribute.String(AttrPolicyID, policyID),
	)
	defer span.End()

	var count int
	err := a.db.QueryRowContext(ctx, queries.FLOW_COUNT_ACTIVE_VIOLATIONS_BY_POLICY, tenantID, policyID).Scan(&count)
	if err != nil {
		observability.RecordError(span, err)
		return 0, err
	}
	return count, nil
}

func (a *PostgresPolicyRepositoryAdapter) ResolveViolation(ctx context.Context, tenantID string, violationID string, expectedVersion int64) error {
	ctx, span := observability.GetTracer().StartSpan(ctx, SpanResolveViolation,
		attribute.String(AttrTenantID, tenantID),
		attribute.String(AttrViolationID, violationID),
	)
	defer span.End()

	now := time.Now().UTC()
	res, err := a.db.ExecContext(ctx, queries.FLOW_RESOLVE_VIOLATION, now, tenantID, violationID, expectedVersion)
	if err != nil {
		observability.RecordError(span, err)
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		observability.RecordError(span, err)
		return err
	}
	if rowsAffected == 0 {
		conflictErr := errors.NewConflictError("violation conflict or not found")
		observability.RecordError(span, conflictErr)
		return conflictErr
	}
	return nil
}

func scanViolationRecord(scanner rowScanner) (*contracts.PolicyViolationRecord, error) {
	var (
		rec         contracts.PolicyViolationRecord
		severity    string
		enforcement string
		status      string
	)

	err := scanner.Scan(
		&rec.ID,
		&rec.PublicID,
		&rec.TenantID,
		&rec.PolicyID,
		&rec.Source,
		&rec.ExternalID,
		&rec.CorrelationID,
		&rec.WorkloadID,
		&rec.Namespace,
		&severity,
		&enforcement,
		&status,
		&rec.Message,
		&rec.DetectionPair,
		&rec.Version,
		&rec.CreatedAt,
		&rec.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	rec.Severity = contracts.PolicySeverity(severity)
	rec.EnforcementPoint = contracts.EnforcementPoint(enforcement)
	rec.Status = contracts.PolicyViolationStatus(status)
	return &rec, nil
}
