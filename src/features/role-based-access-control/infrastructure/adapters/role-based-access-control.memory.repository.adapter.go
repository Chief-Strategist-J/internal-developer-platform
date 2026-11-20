package adapters

import (
	"context"
	"sync"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/shared/errors"
)

type MemoryRevocationAdapter struct {
	mu          sync.RWMutex
	revokedSubs map[string]bool
	revokedJTIs map[string]bool
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

type MemoryAccessGrantAdapter struct {
	mu     sync.RWMutex
	grants map[string]rbacContracts.AccessGrantRecord
}

func NewMemoryAccessGrantAdapter() *MemoryAccessGrantAdapter {
	return &MemoryAccessGrantAdapter{
		grants: make(map[string]rbacContracts.AccessGrantRecord),
	}
}

func (a *MemoryAccessGrantAdapter) SaveGrant(ctx context.Context, grant rbacContracts.AccessGrantRecord) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := grant.TenantID + ":" + grant.ID
	a.grants[key] = grant
	return nil
}

func (a *MemoryAccessGrantAdapter) GetGrant(ctx context.Context, tenantID string, id string) (*rbacContracts.AccessGrantRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	key := tenantID + ":" + id
	g, ok := a.grants[key]
	if !ok {
		return nil, errors.NewNotFoundError("access grant not found")
	}
	return &g, nil
}

func (a *MemoryAccessGrantAdapter) RevokeGrant(ctx context.Context, tenantID string, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := tenantID + ":" + id
	g, ok := a.grants[key]
	if !ok {
		return errors.NewNotFoundError("access grant not found")
	}
	g.Status = "REVOKED"
	g.UpdatedAt = time.Now()
	a.grants[key] = g
	return nil
}

func (a *MemoryAccessGrantAdapter) ListActiveGrants(ctx context.Context, tenantID string, subjectID string) ([]rbacContracts.AccessGrantRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	now := time.Now()
	var res []rbacContracts.AccessGrantRecord
	for _, g := range a.grants {
		if g.TenantID == tenantID && (subjectID == "" || g.SubjectID == subjectID) {
			if g.Status == "ACTIVE" && now.Before(g.ExpiresAt) {
				res = append(res, g)
			}
		}
	}
	return res, nil
}
