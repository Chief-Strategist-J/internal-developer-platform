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
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"internal-developer-platform/src/features/self-service-provisioning/internal/contracts"
	"internal-developer-platform/src/features/self-service-provisioning/internal/domain/rules"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type MemoryProvisioningGovernanceAdapter struct {
	mu           sync.Mutex
	operations   map[string]string
	retentions   map[string]contracts.RetentionDecision
	dnsCooldowns map[string]time.Time
	sagaLocks    map[string]contracts.SagaLockRecord
}

func NewMemoryProvisioningGovernanceAdapter() *MemoryProvisioningGovernanceAdapter {
	return &MemoryProvisioningGovernanceAdapter{
		operations:   make(map[string]string),
		retentions:   make(map[string]contracts.RetentionDecision),
		dnsCooldowns: make(map[string]time.Time),
		sagaLocks:    make(map[string]contracts.SagaLockRecord),
	}
}

func (a *MemoryProvisioningGovernanceAdapter) RecordOperationID(ctx context.Context, intentID string, operationID string) error {
	return observability.WithClientSpan(ctx, "self_service.governance.memory.record_op", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.operations[intentID] = operationID
		return nil
	}, attribute.String("db.system", "memory"), attribute.String("intent.id", intentID))
}

func (a *MemoryProvisioningGovernanceAdapter) RecordRetentionDecision(ctx context.Context, d contracts.RetentionDecision) error {
	return observability.WithClientSpan(ctx, "self_service.governance.memory.record_retention", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.retentions[d.TenantID+"|"+d.ResourceID] = d
		return nil
	}, attribute.String("db.system", "memory"))
}

func (a *MemoryProvisioningGovernanceAdapter) HasRetentionDecision(ctx context.Context, tenantID string, resourceID string) (bool, error) {
	return observability.RunTracedClient(ctx, "self_service.governance.memory.has_retention", func(ctx context.Context) (bool, error) {
		a.mu.Lock()
		defer a.mu.Unlock()
		_, ok := a.retentions[tenantID+"|"+resourceID]
		return ok, nil
	}, attribute.String("db.system", "memory"))
}

func (a *MemoryProvisioningGovernanceAdapter) EnterDNSCooldown(ctx context.Context, tenantID string, dnsName string, cooldown time.Duration) error {
	return observability.WithClientSpan(ctx, "self_service.governance.memory.dns_cooldown", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.dnsCooldowns[rules.NormalizeResourceName(dnsName)] = time.Now().Add(cooldown)
		return nil
	}, attribute.String("db.system", "memory"))
}

func (a *MemoryProvisioningGovernanceAdapter) IsDNSInCooldown(ctx context.Context, dnsName string) (bool, error) {
	return observability.RunTracedClient(ctx, "self_service.governance.memory.check_dns_cooldown", func(ctx context.Context) (bool, error) {
		a.mu.Lock()
		defer a.mu.Unlock()
		until, ok := a.dnsCooldowns[rules.NormalizeResourceName(dnsName)]
		return ok && until.After(time.Now()), nil
	}, attribute.String("db.system", "memory"))
}

func (a *MemoryProvisioningGovernanceAdapter) AcquireSagaLock(ctx context.Context, serviceKey string, tenantID string, operation string) error {
	return observability.WithClientSpan(ctx, "self_service.governance.memory.acquire_saga_lock", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if cur, ok := a.sagaLocks[serviceKey]; ok && cur.State != "COMPLETED" {
			return errors.NewConflictError(fmt.Sprintf("EC-13: service %s already has an active saga lock", serviceKey))
		}
		a.sagaLocks[serviceKey] = contracts.SagaLockRecord{
			ServiceKey:      serviceKey,
			TenantID:        tenantID,
			ActiveOperation: operation,
			State:           "RUNNING",
		}
		return nil
	}, attribute.String("db.system", "memory"))
}

func (a *MemoryProvisioningGovernanceAdapter) ReleaseSagaLock(ctx context.Context, serviceKey string, finalState string) error {
	return observability.WithClientSpan(ctx, "self_service.governance.memory.release_saga_lock", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if cur, ok := a.sagaLocks[serviceKey]; ok {
			cur.State = finalState
			a.sagaLocks[serviceKey] = cur
		}
		return nil
	}, attribute.String("db.system", "memory"))
}

func (a *MemoryProvisioningGovernanceAdapter) RunSweeper(ctx context.Context) (*contracts.SweeperReport, error) {
	return observability.RunTracedClient(ctx, "self_service.governance.memory.sweeper", func(ctx context.Context) (*contracts.SweeperReport, error) {
		return &contracts.SweeperReport{
			UntrackedIntents: make([]string, 0),
			DuplicateTargets: make([]string, 0),
		}, nil
	}, attribute.String("db.system", "memory"))
}
