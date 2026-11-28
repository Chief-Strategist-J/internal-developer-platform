/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING RESERVATION ADAPTERS (V.3 role 11)

 1. Role:
    - Enforces INV-04 atomic reservations, INV-05 IP space quota, INV-15 tenant isolation, and EC-03 unique normalized name 409 conflict.
 2. Invariants:
    - Uses RunTracedClient / WithClientSpan (SpanKindClient); single-transaction PostgreSQL reservation checks.
    - Zero inline comments below this header.
*/
package adapters

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/features/self-service-provisioning/internal/domain/rules"
	"internal-developer-platform/src/features/self-service-provisioning/internal/infrastructure/queries"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type PostgresProvisioningReservationAdapter struct {
	db *sql.DB
}

func NewPostgresProvisioningReservationAdapter(db *sql.DB) *PostgresProvisioningReservationAdapter {
	return &PostgresProvisioningReservationAdapter{db: db}
}

func (a *PostgresProvisioningReservationAdapter) Reserve(ctx context.Context, req contracts.ReservationRequest) (*contracts.ReservationRecord, error) {
	return observability.RunTracedClient(ctx, "self_service.reservation.postgres.reserve", func(ctx context.Context) (*contracts.ReservationRecord, error) {
		if a.db == nil {
			return nil, errors.NewInternalError("provisioning reservation database connection is nil")
		}
		normName := req.NormalizedName
		if normName == "" {
			normName = rules.NormalizeResourceName(req.RawName)
		}
		req.NormalizedName = normName
		ttl := req.TTLSeconds
		if ttl <= 0 {
			ttl = 900
		}

		tx, err := a.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to begin reservation transaction: %w", err)
		}
		defer func() {
			_ = tx.Rollback()
		}()

		if _, err := tx.ExecContext(ctx, queries.ExpireStaleReservationsSQL); err != nil {
			return nil, fmt.Errorf("failed to expire stale reservations: %w", err)
		}

		ceiling := rules.DefaultTenantCapacityCeiling(req.TenantID)
		row := tx.QueryRowContext(ctx, queries.SelectTenantCapacityCeilingSQL, req.TenantID)
		if scanErr := row.Scan(&ceiling.MaxQuotaUnits, &ceiling.MaxIPCIDRBlocks, &ceiling.MaxClusterIPUtilPct); scanErr != nil && !stderrors.Is(scanErr, sql.ErrNoRows) {
			return nil, fmt.Errorf("failed to load tenant capacity ceiling: %w", scanErr)
		}

		var quotaUsed int
		var ipUsed int
		if err := tx.QueryRowContext(ctx, queries.SumTenantActiveReservationsSQL, req.TenantID).Scan(&quotaUsed, &ipUsed); err != nil {
			return nil, fmt.Errorf("failed to sum tenant active reservations: %w", err)
		}

		if err := rules.ValidateCapacityReservation(quotaUsed, ipUsed, req, ceiling); err != nil {
			return nil, err
		}

		resID := fmt.Sprintf("ssp-res-%d", time.Now().UnixNano())
		sum := sha256.Sum256([]byte(resID + ":" + req.TenantID + ":" + normName))
		publicID := "sspr_" + hex.EncodeToString(sum[:13])
		expiresAt := time.Now().Add(time.Duration(ttl) * time.Second)

		if _, err := tx.ExecContext(ctx, queries.InsertProvisioningReservationSQL, resID, publicID, req.TenantID, req.ResourceKind, normName, req.QuotaUnits, req.IPCIDRBlocks, expiresAt); err != nil {
			var pqErr *pq.Error
			if stderrors.As(err, &pqErr) && pqErr.Code == "23505" {
				return nil, errors.NewConflictError(fmt.Sprintf("resource name %q is already reserved", normName))
			}
			return nil, fmt.Errorf("failed to insert reservation: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit reservation: %w", err)
		}

		return &contracts.ReservationRecord{
			ID:             resID,
			PublicID:       publicID,
			TenantID:       req.TenantID,
			ResourceKind:   req.ResourceKind,
			NormalizedName: normName,
			QuotaUnits:     req.QuotaUnits,
			IPCIDRBlocks:   req.IPCIDRBlocks,
			Status:         "RESERVED",
		}, nil
	}, attribute.String("db.system", "postgresql"), attribute.String("tenant.id", req.TenantID))
}

func (a *PostgresProvisioningReservationAdapter) Commit(ctx context.Context, reservationID string) error {
	return observability.WithClientSpan(ctx, "self_service.reservation.postgres.commit", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("provisioning reservation database connection is nil")
		}
		_, err := a.db.ExecContext(ctx, queries.CommitProvisioningReservationSQL, reservationID)
		return err
	}, attribute.String("db.system", "postgresql"), attribute.String("reservation.id", reservationID))
}

func (a *PostgresProvisioningReservationAdapter) Release(ctx context.Context, reservationID string) error {
	return observability.WithClientSpan(ctx, "self_service.reservation.postgres.release", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("provisioning reservation database connection is nil")
		}
		_, err := a.db.ExecContext(ctx, queries.ReleaseProvisioningReservationSQL, reservationID)
		return err
	}, attribute.String("db.system", "postgresql"), attribute.String("reservation.id", reservationID))
}

func (a *PostgresProvisioningReservationAdapter) SetTenantCeiling(ctx context.Context, ceiling contracts.TenantCapacityCeiling) error {
	return observability.WithClientSpan(ctx, "self_service.reservation.postgres.set_ceiling", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("provisioning reservation database connection is nil")
		}
		_, err := a.db.ExecContext(ctx, queries.UpsertTenantCapacityCeilingSQL, ceiling.TenantID, ceiling.MaxQuotaUnits, ceiling.MaxIPCIDRBlocks, ceiling.MaxClusterIPUtilPct)
		return err
	}, attribute.String("db.system", "postgresql"), attribute.String("tenant.id", ceiling.TenantID))
}
