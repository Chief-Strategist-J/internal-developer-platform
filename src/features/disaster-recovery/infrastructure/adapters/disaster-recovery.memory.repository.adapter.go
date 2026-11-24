// Package adapters provides in-memory thread-safe storage implementations for Disaster Recovery.
//
// Architectural Blueprint:
// - Step 11 Adapters: Concrete implementation of ports.DRPlanStorePort.
// - Provides thread-safe, race-free execution for integration tests and local verification.
// - 100% comment-free function bodies.
package adapters

import (
	"context"
	"fmt"
	"sync"
	"time"

	"internal-developer-platform/src/features/disaster-recovery/contracts"
	"internal-developer-platform/src/shared/errors"
)

type MemoryDRStoreAdapter struct {
	mu            sync.RWMutex
	plans         map[string]contracts.DRPlanRecord
	incidents     map[string]contracts.DRIncidentRecord
	failoverOps   map[string]contracts.DRFailoverOperationRecord
	fencingEpochs map[string]contracts.FencingEpochRecord
	drillRuns     map[string]contracts.DrillRunRecord
	failbackOps   map[string]contracts.DRFailbackOperationRecord
}

func NewMemoryDRStoreAdapter() *MemoryDRStoreAdapter {
	return &MemoryDRStoreAdapter{
		plans:         make(map[string]contracts.DRPlanRecord),
		incidents:     make(map[string]contracts.DRIncidentRecord),
		failoverOps:   make(map[string]contracts.DRFailoverOperationRecord),
		fencingEpochs: make(map[string]contracts.FencingEpochRecord),
		drillRuns:     make(map[string]contracts.DrillRunRecord),
		failbackOps:   make(map[string]contracts.DRFailbackOperationRecord),
	}
}

func (a *MemoryDRStoreAdapter) SavePlan(ctx context.Context, plan contracts.DRPlanRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, p := range a.plans {
		if p.TenantID == plan.TenantID && p.IdempotencyKey == plan.IdempotencyKey {
			return errors.NewConflictError("plan with idempotency key already exists")
		}
	}
	a.plans[plan.ID] = plan
	return nil
}

func (a *MemoryDRStoreAdapter) GetPlanByID(ctx context.Context, tenantID string, id string) (*contracts.DRPlanRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, p := range a.plans {
		if p.TenantID == tenantID && (p.ID == id || p.PublicID == id || p.PlanName == id) {
			copyRecord := p
			return &copyRecord, nil
		}
	}
	return nil, errors.NewNotFoundError(fmt.Sprintf("dr plan '%s' not found", id))
}

func (a *MemoryDRStoreAdapter) GetPlanByIdempotencyKey(ctx context.Context, tenantID string, key string) (*contracts.DRPlanRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, p := range a.plans {
		if p.TenantID == tenantID && p.IdempotencyKey == key {
			copyRecord := p
			return &copyRecord, nil
		}
	}
	return nil, errors.NewNotFoundError("dr plan not found")
}

func (a *MemoryDRStoreAdapter) ListPlans(ctx context.Context, tenantID string) ([]contracts.DRPlanRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]contracts.DRPlanRecord, 0)
	for _, p := range a.plans {
		if p.TenantID == tenantID {
			result = append(result, p)
		}
	}
	return result, nil
}

func (a *MemoryDRStoreAdapter) UpdatePlan(ctx context.Context, plan contracts.DRPlanRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	existing, exists := a.plans[plan.ID]
	if !exists {
		return errors.NewNotFoundError("dr plan not found for update")
	}
	if existing.TenantID != plan.TenantID {
		return errors.NewForbiddenError("tenant id mismatch")
	}
	plan.Version = existing.Version + 1
	plan.UpdatedAt = time.Now().UTC()
	a.plans[plan.ID] = plan
	return nil
}

func (a *MemoryDRStoreAdapter) SaveIncident(ctx context.Context, inc contracts.DRIncidentRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.incidents[inc.ID] = inc
	return nil
}

func (a *MemoryDRStoreAdapter) GetIncidentByID(ctx context.Context, tenantID string, id string) (*contracts.DRIncidentRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, inc := range a.incidents {
		if inc.TenantID == tenantID && (inc.ID == id || inc.PublicID == id) {
			copyInc := inc
			return &copyInc, nil
		}
	}
	return nil, errors.NewNotFoundError(fmt.Sprintf("incident '%s' not found", id))
}

func (a *MemoryDRStoreAdapter) UpdateIncident(ctx context.Context, inc contracts.DRIncidentRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.incidents[inc.ID] = inc
	return nil
}

func (a *MemoryDRStoreAdapter) SaveFailoverOp(ctx context.Context, op contracts.DRFailoverOperationRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failoverOps[op.ID] = op
	return nil
}

func (a *MemoryDRStoreAdapter) GetFailoverOpByID(ctx context.Context, tenantID string, id string) (*contracts.DRFailoverOperationRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, op := range a.failoverOps {
		if op.TenantID == tenantID && (op.ID == id || op.PublicID == id) {
			copyOp := op
			return &copyOp, nil
		}
	}
	return nil, errors.NewNotFoundError(fmt.Sprintf("failover operation '%s' not found", id))
}

func (a *MemoryDRStoreAdapter) UpdateFailoverOp(ctx context.Context, op contracts.DRFailoverOperationRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	op.UpdatedAt = time.Now().UTC()
	a.failoverOps[op.ID] = op
	return nil
}

func (a *MemoryDRStoreAdapter) SaveFailbackOp(ctx context.Context, op contracts.DRFailbackOperationRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failbackOps[op.ID] = op
	return nil
}

func (a *MemoryDRStoreAdapter) GetFailbackOpByID(ctx context.Context, tenantID string, id string) (*contracts.DRFailbackOperationRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, op := range a.failbackOps {
		if op.TenantID == tenantID && (op.ID == id || op.PublicID == id) {
			copyOp := op
			return &copyOp, nil
		}
	}
	return nil, errors.NewNotFoundError("failback op not found")
}

func (a *MemoryDRStoreAdapter) UpdateFailbackOp(ctx context.Context, op contracts.DRFailbackOperationRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	op.UpdatedAt = time.Now().UTC()
	a.failbackOps[op.ID] = op
	return nil
}

func (a *MemoryDRStoreAdapter) SaveFencingEpoch(ctx context.Context, epoch contracts.FencingEpochRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.fencingEpochs[epoch.DatasetID] = epoch
	return nil
}

func (a *MemoryDRStoreAdapter) GetFencingEpoch(ctx context.Context, datasetID string) (*contracts.FencingEpochRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	rec, exists := a.fencingEpochs[datasetID]
	if !exists {
		return &contracts.FencingEpochRecord{
			DatasetID: datasetID,
			Authority: "PRIMARY",
			Epoch:     0,
			FencedAt:  time.Now().UTC(),
		}, nil
	}
	copyRec := rec
	return &copyRec, nil
}

func (a *MemoryDRStoreAdapter) IncrementFencingEpoch(ctx context.Context, datasetID string, authority string) (*contracts.FencingEpochRecord, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	rec, exists := a.fencingEpochs[datasetID]
	newEpoch := int64(1)
	if exists {
		newEpoch = rec.Epoch + 1
	}
	updated := contracts.FencingEpochRecord{
		DatasetID: datasetID,
		Authority: authority,
		Epoch:     newEpoch,
		FencedAt:  time.Now().UTC(),
	}
	a.fencingEpochs[datasetID] = updated
	return &updated, nil
}

func (a *MemoryDRStoreAdapter) SaveDrillRun(ctx context.Context, drill contracts.DrillRunRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.drillRuns[drill.ID] = drill
	return nil
}

func (a *MemoryDRStoreAdapter) GetDrillRunByID(ctx context.Context, id string) (*contracts.DrillRunRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	rec, exists := a.drillRuns[id]
	if !exists {
		return nil, errors.NewNotFoundError("drill run not found")
	}
	copyRec := rec
	return &copyRec, nil
}

