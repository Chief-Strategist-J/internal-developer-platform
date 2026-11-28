/*
ALGORITHM & ARCHITECTURE BLUEPRINT: MEMORY TRAFFIC ADAPTER (V.3 role 11, Testing & Conformance)

1. Purpose:
   - In-memory implementation of ports.TrafficPort for fast isolated testing.
2. Invariants:
   - Thread-safe via sync.RWMutex; zero inline comments below header.
*/

package adapters

import (
	"context"
	"fmt"
	"sync"
)

type MemoryTrafficAdapter struct {
	mu      sync.RWMutex
	weights map[string]int
}

func NewMemoryTrafficAdapter() *MemoryTrafficAdapter {
	return &MemoryTrafficAdapter{
		weights: make(map[string]int),
	}
}

func (m *MemoryTrafficAdapter) SetTrafficWeight(_ context.Context, tenantID, workloadID string, weightPct int) error {
	if weightPct < 0 || weightPct > 100 {
		return fmt.Errorf("invalid weight percentage: %d", weightPct)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%s", tenantID, workloadID)
	m.weights[key] = weightPct
	return nil
}

func (m *MemoryTrafficAdapter) GetTrafficWeight(_ context.Context, tenantID, workloadID string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key := fmt.Sprintf("%s:%s", tenantID, workloadID)
	return m.weights[key], nil
}
