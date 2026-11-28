/*
ALGORITHM & ARCHITECTURE BLUEPRINT: MEMORY REPOSITORY ADAPTER (V.3 role 11, Testing & Conformance)

1. Purpose:
   - In-memory implementation of ports.RepositoryPort for fast unit testing and FSM validation.
   - Enforces same optimistic locking and constraint validations in-memory.
2. Invariants:
   - Thread-safe via sync.RWMutex; zero inline comments below header.
*/

package adapters

import (
	"context"
	"fmt"
	"sync"
	"time"

	"internal-developer-platform/src/features/deployment-strategies/internal/contracts"
	"internal-developer-platform/src/shared/errors"
)

type MemoryRepositoryAdapter struct {
	mu           sync.RWMutex
	plans        map[string]contracts.RolloutPlan
	executions   map[string]contracts.RolloutExecution
	analysisRuns []contracts.RolloutAnalysisRun
}

func NewMemoryRepositoryAdapter() *MemoryRepositoryAdapter {
	return &MemoryRepositoryAdapter{
		plans:      make(map[string]contracts.RolloutPlan),
		executions: make(map[string]contracts.RolloutExecution),
	}
}

func (m *MemoryRepositoryAdapter) CreatePlan(_ context.Context, plan contracts.RolloutPlan) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", plan.TenantID, plan.ID)
	if _, exists := m.plans[key]; exists {
		return errors.NewConflictError("plan already exists")
	}
	m.plans[key] = plan
	return nil
}

func (m *MemoryRepositoryAdapter) GetPlan(_ context.Context, tenantID, planID string) (*contracts.RolloutPlan, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", tenantID, planID)
	plan, exists := m.plans[key]
	if !exists {
		return nil, errors.NewNotFoundError("plan not found")
	}
	copied := plan
	return &copied, nil
}

func (m *MemoryRepositoryAdapter) CreateExecution(_ context.Context, execution contracts.RolloutExecution) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", execution.TenantID, execution.ID)
	if _, exists := m.executions[key]; exists {
		return errors.NewConflictError("execution already exists")
	}
	m.executions[key] = execution
	return nil
}

func (m *MemoryRepositoryAdapter) GetExecution(_ context.Context, tenantID, executionID string) (*contracts.RolloutExecution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", tenantID, executionID)
	exec, exists := m.executions[key]
	if !exists {
		return nil, errors.NewNotFoundError("execution not found")
	}
	copied := exec
	return &copied, nil
}

func (m *MemoryRepositoryAdapter) GetActiveExecutionForWorkload(_ context.Context, tenantID, workloadID string) (*contracts.RolloutExecution, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, exec := range m.executions {
		if exec.TenantID == tenantID && exec.WorkloadID == workloadID {
			if exec.Status != contracts.RolloutStatusCompleted && exec.Status != contracts.RolloutStatusAborted {
				copied := exec
				return &copied, nil
			}
		}
	}
	return nil, nil
}

func (m *MemoryRepositoryAdapter) UpdateExecutionStatus(_ context.Context, tenantID, executionID string, status contracts.RolloutStatus, stepIndex, weightPct int, errorMsg string, expectedVersion int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", tenantID, executionID)
	exec, exists := m.executions[key]
	if !exists {
		return errors.NewNotFoundError("execution not found")
	}
	if exec.Version != expectedVersion {
		return errors.NewConflictError("optimistic concurrency conflict: version mismatch")
	}
	exec.Status = status
	exec.CurrentStepIndex = stepIndex
	exec.CurrentWeightPct = weightPct
	exec.ErrorMessage = errorMsg
	exec.Version++
	exec.UpdatedAt = time.Now().UTC()
	m.executions[key] = exec
	return nil
}

func (m *MemoryRepositoryAdapter) RecordAnalysisRun(_ context.Context, run contracts.RolloutAnalysisRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.analysisRuns = append(m.analysisRuns, run)
	return nil
}
