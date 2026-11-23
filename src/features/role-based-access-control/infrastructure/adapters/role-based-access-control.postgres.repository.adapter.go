// Package adapters provides outbound persistence adapters for Role-Based Access Control.
//
// Architectural Blueprint:
// - Step 11 Adapter: PostgreSQL persistence adapter for access grants and user role bindings.
// - Traced under OpenTelemetry child spans with error recording.
// - 100% comment-free function bodies.
package adapters

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	rbacQueries "internal-developer-platform/src/features/role-based-access-control/infrastructure/queries"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type PostgresRBACAdapter struct {
	db *sql.DB
}

func NewPostgresRBACAdapter(db *sql.DB) *PostgresRBACAdapter {
	return &PostgresRBACAdapter{db: db}
}

func (a *PostgresRBACAdapter) SaveGrant(ctx context.Context, grant rbacContracts.AccessGrantRecord) error {
	return observability.WithSpan(ctx, "postgres.rbac.save_grant", func(ctx context.Context) error {
		_, err := a.db.ExecContext(
			ctx,
			rbacQueries.FlowInsertAccessGrant,
			grant.ID,
			grant.TenantID,
			grant.SubjectID,
			grant.TargetNamespace,
			grant.Role,
			grant.Reason,
			grant.DurationSeconds,
			grant.ApprovedBy,
			grant.Status,
			grant.ExpiresAt,
			grant.IdempotencyKey,
			grant.Version,
			grant.CreatedAt,
			grant.UpdatedAt,
		)
		if err != nil {
			if pqErr, ok := err.(*pq.Error); ok {
				if pqErr.Code == "23505" {
					if strings.Contains(pqErr.Constraint, "idempotency") {
						return nil
					}
					return errors.NewConflictError("access grant already exists")
				}
			}
			return fmt.Errorf("failed to save access grant: %w", err)
		}
		return nil
	}, attribute.String("tenant_id", grant.TenantID), attribute.String("grant_id", grant.ID), attribute.String("subject_id", grant.SubjectID))
}

func (a *PostgresRBACAdapter) GetGrant(ctx context.Context, tenantID string, id string) (*rbacContracts.AccessGrantRecord, error) {
	return observability.RunTraced(ctx, "postgres.rbac.get_grant", func(ctx context.Context) (*rbacContracts.AccessGrantRecord, error) {
		var g rbacContracts.AccessGrantRecord
		err := a.db.QueryRowContext(ctx, rbacQueries.FlowGetAccessGrantByID, tenantID, id).Scan(
			&g.ID,
			&g.TenantID,
			&g.SubjectID,
			&g.TargetNamespace,
			&g.Role,
			&g.Reason,
			&g.DurationSeconds,
			&g.ApprovedBy,
			&g.Status,
			&g.ExpiresAt,
			&g.IdempotencyKey,
			&g.Version,
			&g.CreatedAt,
			&g.UpdatedAt,
		)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, errors.NewNotFoundError("access grant not found")
			}
			return nil, fmt.Errorf("failed to query access grant: %w", err)
		}
		return &g, nil
	}, attribute.String("tenant_id", tenantID), attribute.String("grant_id", id))
}

func (a *PostgresRBACAdapter) RevokeGrant(ctx context.Context, tenantID string, id string) error {
	return observability.WithSpan(ctx, "postgres.rbac.revoke_grant", func(ctx context.Context) error {
		res, err := a.db.ExecContext(ctx, rbacQueries.FlowRevokeAccessGrant, tenantID, id, time.Now())
		if err != nil {
			return fmt.Errorf("failed to revoke access grant: %w", err)
		}
		rowsAffected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected: %w", err)
		}
		if rowsAffected == 0 {
			return errors.NewNotFoundError("access grant not found or already revoked")
		}
		return nil
	}, attribute.String("tenant_id", tenantID), attribute.String("grant_id", id))
}

func (a *PostgresRBACAdapter) ListActiveGrants(ctx context.Context, tenantID string, subjectID string) ([]rbacContracts.AccessGrantRecord, error) {
	return observability.RunTraced(ctx, "postgres.rbac.list_active_grants", func(ctx context.Context) ([]rbacContracts.AccessGrantRecord, error) {
		rows, err := a.db.QueryContext(ctx, rbacQueries.FlowListActiveAccessGrants, tenantID, subjectID, time.Now())
		if err != nil {
			return nil, fmt.Errorf("failed to list active access grants: %w", err)
		}
		defer rows.Close()

		var results []rbacContracts.AccessGrantRecord
		for rows.Next() {
			var g rbacContracts.AccessGrantRecord
			if err := rows.Scan(
				&g.ID,
				&g.TenantID,
				&g.SubjectID,
				&g.TargetNamespace,
				&g.Role,
				&g.Reason,
				&g.DurationSeconds,
				&g.ApprovedBy,
				&g.Status,
				&g.ExpiresAt,
				&g.IdempotencyKey,
				&g.Version,
				&g.CreatedAt,
				&g.UpdatedAt,
			); err != nil {
				return nil, fmt.Errorf("failed to scan access grant: %w", err)
			}
			results = append(results, g)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("rows iteration error: %w", err)
		}
		if results == nil {
			results = []rbacContracts.AccessGrantRecord{}
		}
		return results, nil
	}, attribute.String("tenant_id", tenantID), attribute.String("subject_id", subjectID))
}

func (a *PostgresRBACAdapter) BindUserRole(ctx context.Context, binding rbacContracts.UserBindingRecord) error {
	return observability.WithSpan(ctx, "postgres.rbac.bind_user_role", func(ctx context.Context) error {
		createdAt := binding.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		_, err := a.db.ExecContext(ctx, rbacQueries.FlowInsertUserBinding, binding.SubjectID, binding.TenantID, string(binding.RoleName), createdAt)
		if err != nil {
			if pqErr, ok := err.(*pq.Error); ok {
				if pqErr.Code == "23503" {
					return errors.NewValidationError(fmt.Sprintf("role %s does not exist", binding.RoleName))
				}
			}
			return fmt.Errorf("failed to bind user role: %w", err)
		}
		return nil
	}, attribute.String("tenant_id", binding.TenantID), attribute.String("subject_id", binding.SubjectID), attribute.String("role", string(binding.RoleName)))
}

func (a *PostgresRBACAdapter) UnbindUserRole(ctx context.Context, tenantID string, subjectID string, role rbacContracts.RoleName) error {
	return observability.WithSpan(ctx, "postgres.rbac.unbind_user_role", func(ctx context.Context) error {
		res, err := a.db.ExecContext(ctx, rbacQueries.FlowDeleteUserBinding, tenantID, subjectID, string(role))
		if err != nil {
			return fmt.Errorf("failed to unbind user role: %w", err)
		}
		rowsAffected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to get rows affected: %w", err)
		}
		if rowsAffected == 0 {
			return errors.NewNotFoundError("user binding not found")
		}
		return nil
	}, attribute.String("tenant_id", tenantID), attribute.String("subject_id", subjectID), attribute.String("role", string(role)))
}

func (a *PostgresRBACAdapter) GetUserRoles(ctx context.Context, tenantID string, subjectID string) ([]rbacContracts.RoleName, error) {
	return observability.RunTraced(ctx, "postgres.rbac.get_user_roles", func(ctx context.Context) ([]rbacContracts.RoleName, error) {
		rows, err := a.db.QueryContext(ctx, rbacQueries.FlowGetUserRoles, tenantID, subjectID)
		if err != nil {
			return nil, fmt.Errorf("failed to query user roles: %w", err)
		}
		defer rows.Close()

		var roles []rbacContracts.RoleName
		for rows.Next() {
			var r string
			if err := rows.Scan(&r); err != nil {
				return nil, fmt.Errorf("failed to scan role name: %w", err)
			}
			roles = append(roles, rbacContracts.RoleName(r))
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("rows iteration error: %w", err)
		}
		if roles == nil {
			roles = []rbacContracts.RoleName{}
		}
		return roles, nil
	}, attribute.String("tenant_id", tenantID), attribute.String("subject_id", subjectID))
}
