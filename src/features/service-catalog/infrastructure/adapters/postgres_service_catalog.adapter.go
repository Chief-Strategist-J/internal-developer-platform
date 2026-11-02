package adapters

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	catalogContracts "internal-developer-platform/src/features/service-catalog/contracts"
	"internal-developer-platform/src/shared/errors"
)

type PostgresServiceCatalogAdapter struct {
	db *sql.DB
}

func NewPostgresServiceCatalogAdapter(db *sql.DB) *PostgresServiceCatalogAdapter {
	return &PostgresServiceCatalogAdapter{db: db}
}

func (a *PostgresServiceCatalogAdapter) GetServiceByID(ctx context.Context, tenantID string, id string) (*catalogContracts.ServiceDescriptor, error) {
	query := `
		SELECT id, tenant_id, name, owner, lifecycle, status, version, created_at, updated_at
		FROM service_descriptors
		WHERE tenant_id = $1 AND id = $2;
	`
	var (
		svc       catalogContracts.ServiceDescriptor
		createdAt time.Time
		updatedAt time.Time
	)

	err := a.db.QueryRowContext(ctx, query, tenantID, id).Scan(
		&svc.ID,
		&svc.TenantID,
		&svc.Name,
		&svc.Owner,
		&svc.Lifecycle,
		&svc.Status,
		&svc.Version,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NewNotFoundError("service not found in catalog")
		}
		return nil, fmt.Errorf("database query error: %w", err)
	}

	svc.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	svc.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return &svc, nil
}

func (a *PostgresServiceCatalogAdapter) RegisterService(ctx context.Context, service *catalogContracts.ServiceDescriptor) error {
	query := `
		INSERT INTO service_descriptors (id, tenant_id, name, owner, lifecycle, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);
	`
	now := time.Now().UTC()
	service.CreatedAt = now.Format(time.RFC3339)
	service.UpdatedAt = now.Format(time.RFC3339)
	if service.Version == 0 {
		service.Version = 1
	}

	_, err := a.db.ExecContext(ctx, query,
		service.ID,
		service.TenantID,
		service.Name,
		service.Owner,
		service.Lifecycle,
		service.Status,
		service.Version,
		now,
		now,
	)
	if err != nil {
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
			return errors.NewConflictError("service already exists in catalog")
		}
		if strings.Contains(err.Error(), "duplicate key value") || strings.Contains(err.Error(), "unique constraint") {
			return errors.NewConflictError("service already exists in catalog")
		}
		return fmt.Errorf("failed to insert service descriptor into postgres: %w", err)
	}

	return nil
}
