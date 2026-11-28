/*
ALGORITHM & ARCHITECTURE BLUEPRINT: SELF-SERVICE PROVISIONING ADAPTER (V.3 role 11, Invariant A)

 1. Role:
    - Concrete adapter implementation separated per V.4 Invariant A.
 2. Invariants:
    - Zero inline comments below this header.
*/
package adapters

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/features/self-service-provisioning/internal/domain/rules"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

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
