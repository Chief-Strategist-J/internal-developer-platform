/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SHARED POOL & FEATURE ROLE ISOLATION (VII.1, II.3)

 1. Role:
    - Enforces per-transaction SET LOCAL ROLE <svc_feature>, search_path isolation, and RLS tenant binding
    on a single shared *sql.DB connection pool (D-01, D-02).
 2. Invariants:
    - Zero per-feature connection pools (VII.1); zero inline comments inside function bodies (C-01).
*/
package db

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
)

var validSQLIdentifier = regexp.MustCompile(`^[a-z][a-z0-9_]{1,62}$`)

type FeatureTxScope struct {
	Schema   string
	Role     string
	TenantID string
}

func ValidateIdentifier(ident string) error {
	if !validSQLIdentifier.MatchString(ident) {
		return fmt.Errorf("invalid postgres identifier %q", ident)
	}
	return nil
}

func BuildSessionPreludeStatements(scope FeatureTxScope) ([]string, error) {
	if err := ValidateIdentifier(scope.Schema); err != nil {
		return nil, fmt.Errorf("invalid schema: %w", err)
	}
	if err := ValidateIdentifier(scope.Role); err != nil {
		return nil, fmt.Errorf("invalid role: %w", err)
	}
	stmts := []string{
		"SET LOCAL lock_timeout = '3s'",
		"SET LOCAL statement_timeout = '30s'",
		fmt.Sprintf("SET LOCAL ROLE %s", scope.Role),
		fmt.Sprintf("SET LOCAL search_path TO %s, public", scope.Schema),
	}
	return stmts, nil
}

func BeginFeatureTx(ctx context.Context, db *sql.DB, scope FeatureTxScope) (*sql.Tx, error) {
	if db == nil {
		return nil, fmt.Errorf("database pool is required")
	}
	stmts, err := BuildSessionPreludeStatements(scope)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, stmt := range stmts {
		if _, execErr := tx.ExecContext(ctx, stmt); execErr != nil {
			_ = tx.Rollback()
			return nil, execErr
		}
	}
	if scope.TenantID != "" {
		if _, tenantErr := tx.ExecContext(ctx, "SELECT set_config('app.current_tenant_id', $1, true)", scope.TenantID); tenantErr != nil {
			_ = tx.Rollback()
			return nil, tenantErr
		}
	}
	return tx, nil
}
