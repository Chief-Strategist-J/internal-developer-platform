package adapters

import (
	"context"
	"sync"
)

type MemoryRevocationAdapter struct {
	mu            sync.RWMutex
	revokedSubs   map[string]bool
	revokedJTIs   map[string]bool
}

func NewMemoryRevocationAdapter() *MemoryRevocationAdapter {
	return &MemoryRevocationAdapter{
		revokedSubs: make(map[string]bool),
		revokedJTIs: make(map[string]bool),
	}
}

func (a *MemoryRevocationAdapter) IsRevoked(ctx context.Context, subjectID string, jti string) (bool, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if subjectID != "" && a.revokedSubs[subjectID] {
		return true, nil
	}
	if jti != "" && a.revokedJTIs[jti] {
		return true, nil
	}
	return false, nil
}

func (a *MemoryRevocationAdapter) RevokeSubject(ctx context.Context, subjectID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.revokedSubs[subjectID] = true
	return nil
}

func (a *MemoryRevocationAdapter) RevokeJTI(ctx context.Context, jti string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.revokedJTIs[jti] = true
	return nil
}
