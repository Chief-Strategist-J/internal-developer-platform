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
	"sync"
	"time"

	"github.com/lib/pq"
	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/features/self-service-provisioning/internal/domain/rules"
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

		const expireStmt = `
			UPDATE self_service_provisioning.provisioning_reservations
			SET status = 'EXPIRED', updated_at = NOW()
			WHERE status = 'RESERVED' AND expires_at <= NOW()
		`
		if _, err := tx.ExecContext(ctx, expireStmt); err != nil {
			return nil, fmt.Errorf("failed to expire stale reservations: %w", err)
		}

		ceiling := rules.DefaultTenantCapacityCeiling(req.TenantID)
		const ceilingQuery = `
			SELECT max_quota_units, max_ip_cidr_blocks, max_cluster_ip_util_pct
			FROM self_service_provisioning.tenant_capacity_ceilings
			WHERE tenant_id = $1
		`
		row := tx.QueryRowContext(ctx, ceilingQuery, req.TenantID)
		if scanErr := row.Scan(&ceiling.MaxQuotaUnits, &ceiling.MaxIPCIDRBlocks, &ceiling.MaxClusterIPUtilPct); scanErr != nil && !stderrors.Is(scanErr, sql.ErrNoRows) {
			return nil, fmt.Errorf("failed to load tenant capacity ceiling: %w", scanErr)
		}

		var quotaUsed int
		var ipUsed int
		const usageQuery = `
			SELECT COALESCE(SUM(quota_units), 0), COALESCE(SUM(ip_cidr_blocks), 0)
			FROM self_service_provisioning.provisioning_reservations
			WHERE tenant_id = $1 AND status IN ('RESERVED', 'COMMITTED')
		`
		if err := tx.QueryRowContext(ctx, usageQuery, req.TenantID).Scan(&quotaUsed, &ipUsed); err != nil {
			return nil, fmt.Errorf("failed to sum tenant active reservations: %w", err)
		}

		if err := rules.ValidateCapacityReservation(quotaUsed, ipUsed, req, ceiling); err != nil {
			return nil, err
		}

		resID := fmt.Sprintf("ssp-res-%d", time.Now().UnixNano())
		sum := sha256.Sum256([]byte(resID + ":" + req.TenantID + ":" + normName))
		publicID := "sspr_" + hex.EncodeToString(sum[:13])
		expiresAt := time.Now().Add(time.Duration(ttl) * time.Second)

		const insertStmt = `
			INSERT INTO self_service_provisioning.provisioning_reservations (
				id, public_id, tenant_id, resource_kind, normalized_name, quota_units, ip_cidr_blocks, status, expires_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, 'RESERVED', $8)
		`
		if _, err := tx.ExecContext(ctx, insertStmt, resID, publicID, req.TenantID, req.ResourceKind, normName, req.QuotaUnits, req.IPCIDRBlocks, expiresAt); err != nil {
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
		const stmt = `
			UPDATE self_service_provisioning.provisioning_reservations
			SET status = 'COMMITTED', updated_at = NOW()
			WHERE id = $1 AND status = 'RESERVED'
		`
		_, err := a.db.ExecContext(ctx, stmt, reservationID)
		return err
	}, attribute.String("db.system", "postgresql"), attribute.String("reservation.id", reservationID))
}

func (a *PostgresProvisioningReservationAdapter) Release(ctx context.Context, reservationID string) error {
	return observability.WithClientSpan(ctx, "self_service.reservation.postgres.release", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("provisioning reservation database connection is nil")
		}
		const stmt = `
			UPDATE self_service_provisioning.provisioning_reservations
			SET status = 'RELEASED', updated_at = NOW()
			WHERE id = $1 AND status = 'RESERVED'
		`
		_, err := a.db.ExecContext(ctx, stmt, reservationID)
		return err
	}, attribute.String("db.system", "postgresql"), attribute.String("reservation.id", reservationID))
}

func (a *PostgresProvisioningReservationAdapter) SetTenantCeiling(ctx context.Context, ceiling contracts.TenantCapacityCeiling) error {
	return observability.WithClientSpan(ctx, "self_service.reservation.postgres.set_ceiling", func(ctx context.Context) error {
		if a.db == nil {
			return errors.NewInternalError("provisioning reservation database connection is nil")
		}
		const stmt = `
			INSERT INTO self_service_provisioning.tenant_capacity_ceilings (
				tenant_id, max_quota_units, max_ip_cidr_blocks, max_cluster_ip_util_pct
			) VALUES ($1, $2, $3, $4)
			ON CONFLICT (tenant_id) DO UPDATE SET
				max_quota_units = EXCLUDED.max_quota_units,
				max_ip_cidr_blocks = EXCLUDED.max_ip_cidr_blocks,
				max_cluster_ip_util_pct = EXCLUDED.max_cluster_ip_util_pct,
				updated_at = NOW()
		`
		_, err := a.db.ExecContext(ctx, stmt, ceiling.TenantID, ceiling.MaxQuotaUnits, ceiling.MaxIPCIDRBlocks, ceiling.MaxClusterIPUtilPct)
		return err
	}, attribute.String("db.system", "postgresql"), attribute.String("tenant.id", ceiling.TenantID))
}

type memoryReservationEntry struct {
	record    contracts.ReservationRecord
	expiresAt time.Time
}

type MemoryProvisioningReservationAdapter struct {
	mu           sync.Mutex
	reservations map[string]memoryReservationEntry
	ceilings     map[string]contracts.TenantCapacityCeiling
}

func NewMemoryProvisioningReservationAdapter() *MemoryProvisioningReservationAdapter {
	return &MemoryProvisioningReservationAdapter{
		reservations: make(map[string]memoryReservationEntry),
		ceilings:     make(map[string]contracts.TenantCapacityCeiling),
	}
}

func (a *MemoryProvisioningReservationAdapter) Reserve(ctx context.Context, req contracts.ReservationRequest) (*contracts.ReservationRecord, error) {
	return observability.RunTracedClient(ctx, "self_service.reservation.memory.reserve", func(ctx context.Context) (*contracts.ReservationRecord, error) {
		a.mu.Lock()
		defer a.mu.Unlock()

		now := time.Now()
		normName := req.NormalizedName
		if normName == "" {
			normName = rules.NormalizeResourceName(req.RawName)
		}
		req.NormalizedName = normName
		ttl := req.TTLSeconds
		if ttl <= 0 {
			ttl = 900
		}

		ceiling, ok := a.ceilings[req.TenantID]
		if !ok {
			ceiling = rules.DefaultTenantCapacityCeiling(req.TenantID)
		}

		quotaUsed := 0
		ipUsed := 0
		for id, entry := range a.reservations {
			if entry.record.Status == "RESERVED" && !entry.expiresAt.After(now) {
				entry.record.Status = "EXPIRED"
				a.reservations[id] = entry
				continue
			}
			if entry.record.Status != "RESERVED" && entry.record.Status != "COMMITTED" {
				continue
			}
			if entry.record.ResourceKind == req.ResourceKind && entry.record.NormalizedName == normName {
				return nil, errors.NewConflictError(fmt.Sprintf("resource name %q is already reserved", normName))
			}
			if entry.record.TenantID == req.TenantID {
				quotaUsed += entry.record.QuotaUnits
				ipUsed += entry.record.IPCIDRBlocks
			}
		}

		if err := rules.ValidateCapacityReservation(quotaUsed, ipUsed, req, ceiling); err != nil {
			return nil, err
		}

		resID := fmt.Sprintf("ssp-res-%d", now.UnixNano())
		sum := sha256.Sum256([]byte(resID + ":" + req.TenantID + ":" + normName))
		publicID := "sspr_" + hex.EncodeToString(sum[:13])
		rec := contracts.ReservationRecord{
			ID:             resID,
			PublicID:       publicID,
			TenantID:       req.TenantID,
			ResourceKind:   req.ResourceKind,
			NormalizedName: normName,
			QuotaUnits:     req.QuotaUnits,
			IPCIDRBlocks:   req.IPCIDRBlocks,
			Status:         "RESERVED",
		}
		a.reservations[resID] = memoryReservationEntry{
			record:    rec,
			expiresAt: now.Add(time.Duration(ttl) * time.Second),
		}
		return &rec, nil
	}, attribute.String("db.system", "memory"), attribute.String("tenant.id", req.TenantID))
}

func (a *MemoryProvisioningReservationAdapter) Commit(ctx context.Context, reservationID string) error {
	return observability.WithClientSpan(ctx, "self_service.reservation.memory.commit", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		entry, ok := a.reservations[reservationID]
		if ok && entry.record.Status == "RESERVED" {
			entry.record.Status = "COMMITTED"
			a.reservations[reservationID] = entry
		}
		return nil
	}, attribute.String("db.system", "memory"), attribute.String("reservation.id", reservationID))
}

func (a *MemoryProvisioningReservationAdapter) Release(ctx context.Context, reservationID string) error {
	return observability.WithClientSpan(ctx, "self_service.reservation.memory.release", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		entry, ok := a.reservations[reservationID]
		if ok && entry.record.Status == "RESERVED" {
			entry.record.Status = "RELEASED"
			a.reservations[reservationID] = entry
		}
		return nil
	}, attribute.String("db.system", "memory"), attribute.String("reservation.id", reservationID))
}

func (a *MemoryProvisioningReservationAdapter) SetTenantCeiling(ctx context.Context, ceiling contracts.TenantCapacityCeiling) error {
	return observability.WithClientSpan(ctx, "self_service.reservation.memory.set_ceiling", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.ceilings[ceiling.TenantID] = ceiling
		return nil
	}, attribute.String("db.system", "memory"), attribute.String("tenant.id", ceiling.TenantID))
}
