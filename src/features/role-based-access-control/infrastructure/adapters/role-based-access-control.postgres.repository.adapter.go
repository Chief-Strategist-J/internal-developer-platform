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
	"encoding/json"
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

type txContextKeyType struct{}

var rbacTxContextKey = txContextKeyType{}

func WithTx(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, rbacTxContextKey, tx)
}

func TxFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(rbacTxContextKey).(*sql.Tx)
	return tx, ok && tx != nil
}

type PostgresRBACAdapter struct {
	db *sql.DB
}

func NewPostgresRBACAdapter(db *sql.DB) *PostgresRBACAdapter {
	return &PostgresRBACAdapter{db: db}
}

func (a *PostgresRBACAdapter) Run(ctx context.Context, fn func(txCtx context.Context) error) error {
	return observability.WithSpan(ctx, "postgres.rbac.unit_of_work", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("database connection is nil for unit of work")
		}
		if _, alreadyInTx := TxFromContext(ctx); alreadyInTx {
			return fn(ctx)
		}
		tx, err := a.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		txCtx := WithTx(ctx, tx)
		if err := fn(txCtx); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
		return nil
	})
}

func (a *PostgresRBACAdapter) exec(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	if tx, ok := TxFromContext(ctx); ok {
		return tx.ExecContext(ctx, query, args...)
	}
	return a.db.ExecContext(ctx, query, args...)
}

func (a *PostgresRBACAdapter) SaveGrant(ctx context.Context, grant rbacContracts.AccessGrantRecord) error {
	return observability.WithSpan(ctx, "postgres.rbac.save_grant", func(ctx context.Context) error {
		_, err := a.exec(
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
		res, err := a.exec(ctx, rbacQueries.FlowRevokeAccessGrant, tenantID, id, time.Now())
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
		_, err := a.exec(ctx, rbacQueries.FlowInsertUserBinding, binding.SubjectID, binding.TenantID, string(binding.RoleName), createdAt)
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
		res, err := a.exec(ctx, rbacQueries.FlowDeleteUserBinding, tenantID, subjectID, string(role))
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

func (a *PostgresRBACAdapter) ClaimExclusiveRoleBinding(ctx context.Context, tenantID string, subjectID string, role rbacContracts.RoleName) error {
	return observability.WithSpan(ctx, "postgres.rbac.claim_exclusive_role_binding", func(ctx context.Context) error {
		res, err := a.db.ExecContext(ctx, rbacQueries.FLOW_CLAIM_EXCLUSIVE_ROLE_BINDING, subjectID, tenantID, string(role), time.Now().UTC())
		if err != nil {
			return fmt.Errorf("failed to claim exclusive role binding: %w", err)
		}
		rowsAffected, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("failed to inspect claimed rows: %w", err)
		}
		if rowsAffected == 0 {
			return errors.NewConflictError("role binding already claimed")
		}
		return nil
	}, attribute.String("tenant_id", tenantID), attribute.String("subject_id", subjectID), attribute.String("role", string(role)))
}

func (a *PostgresRBACAdapter) SaveRoleHierarchyEdge(ctx context.Context, parentRole string, childRole string) error {
	return observability.WithSpan(ctx, "postgres.rbac.save_role_hierarchy_edge", func(ctx context.Context) error {
		_, err := a.db.ExecContext(ctx, rbacQueries.FLOW_INSERT_ROLE_HIERARCHY, parentRole, childRole, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("failed to save role hierarchy edge: %w", err)
		}
		return nil
	}, attribute.String("parent_role", parentRole), attribute.String("child_role", childRole))
}

func (a *PostgresRBACAdapter) GetRoleHierarchyEdges(ctx context.Context) (map[string][]string, error) {
	return observability.RunTraced(ctx, "postgres.rbac.get_role_hierarchy_edges", func(ctx context.Context) (map[string][]string, error) {
		rows, err := a.db.QueryContext(ctx, rbacQueries.FLOW_GET_ROLE_HIERARCHY)
		if err != nil {
			return nil, fmt.Errorf("failed to query role hierarchy: %w", err)
		}
		defer rows.Close()
		edges := make(map[string][]string)
		for rows.Next() {
			var parent, child string
			if err := rows.Scan(&parent, &child); err != nil {
				return nil, fmt.Errorf("failed to scan role hierarchy edge: %w", err)
			}
			edges[parent] = append(edges[parent], child)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("rows iteration error: %w", err)
		}
		return edges, nil
	})
}

func (a *PostgresRBACAdapter) LoadRolePermissions(ctx context.Context) (map[rbacContracts.RoleName][]rbacContracts.Permission, error) {
	return observability.RunTraced(ctx, "postgres.rbac.load_role_permissions", func(ctx context.Context) (map[rbacContracts.RoleName][]rbacContracts.Permission, error) {
		rows, err := a.db.QueryContext(ctx, rbacQueries.FLOW_GET_ALL_ROLE_DEFINITIONS)
		if err != nil {
			return nil, fmt.Errorf("failed to query role definitions: %w", err)
		}
		defer rows.Close()
		defs := make(map[rbacContracts.RoleName][]rbacContracts.Permission)
		for rows.Next() {
			var name string
			var rawPerms []byte
			if err := rows.Scan(&name, &rawPerms); err != nil {
				return nil, fmt.Errorf("failed to scan role definition: %w", err)
			}
			var perms []rbacContracts.Permission
			if len(rawPerms) > 0 {
				if err := json.Unmarshal(rawPerms, &perms); err != nil {
					return nil, fmt.Errorf("failed to unmarshal permissions for role %s: %w", name, err)
				}
			}
			defs[rbacContracts.RoleName(name)] = perms
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("rows iteration error: %w", err)
		}
		return defs, nil
	})
}
