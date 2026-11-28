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
	"sync"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type MemoryProvisioningLedgerAdapter struct {
	mu      sync.Mutex
	Intents map[string]contracts.ProvisioningIntent
}

func NewMemoryProvisioningLedgerAdapter() *MemoryProvisioningLedgerAdapter {
	return &MemoryProvisioningLedgerAdapter{
		Intents: make(map[string]contracts.ProvisioningIntent),
	}
}

func (a *MemoryProvisioningLedgerAdapter) RecordIntent(ctx context.Context, intent contracts.ProvisioningIntent) error {
	return observability.WithClientSpan(ctx, "self_service.ledger.memory.record_intent", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.Intents[intent.ID] = intent
		return nil
	}, attribute.String("db.system", "memory"), attribute.String("intent.id", intent.ID))
}

func (a *MemoryProvisioningLedgerAdapter) RecordOutcome(ctx context.Context, intentID string, attempt int, status string, outcomeID string, errMsg string) error {
	return observability.WithClientSpan(ctx, "self_service.ledger.memory.record_outcome", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		it, ok := a.Intents[intentID]
		if !ok || it.Attempt != attempt {
			return errors.NewConflictError("stale attempt fencing conflict on provisioning intent")
		}
		it.Status = status
		it.OutcomeID = outcomeID
		it.ErrorMessage = errMsg
		a.Intents[intentID] = it
		return nil
	}, attribute.String("db.system", "memory"), attribute.String("intent.id", intentID))
}

func (a *MemoryProvisioningLedgerAdapter) ListIntents(ctx context.Context, tenantID string, limit int) ([]contracts.ProvisioningIntent, error) {
	return observability.RunTracedClient(ctx, "self_service.ledger.memory.list_intents", func(ctx context.Context) ([]contracts.ProvisioningIntent, error) {
		a.mu.Lock()
		defer a.mu.Unlock()
		items := make([]contracts.ProvisioningIntent, 0)
		for _, it := range a.Intents {
			if it.TenantID == tenantID {
				items = append(items, it)
				if limit > 0 && len(items) >= limit {
					break
				}
			}
		}
		return items, nil
	}, attribute.String("db.system", "memory"))
}
