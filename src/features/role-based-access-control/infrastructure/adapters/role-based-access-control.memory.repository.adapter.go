/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC MEMORY REPOSITORY ADAPTER

1. Architecture Role:
   - Outbound infrastructure adapter implementing RevocationRepositoryPort, AccessGrantStorePort, and UserBindingRepositoryPort.
   - Hermetic thread-safe memory stores for unit/integration testing and runtime fallback.
   - Enforces Tier-4 infrastructure spans for all operations.
   - Zero inline comments inside method bodies.
*/
package adapters

import (
	"context"
	"sync"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/infra/observability"
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
	return observability.RunTraced(ctx, "memory.revocation.is_revoked", func(ctx context.Context) (bool, error) {
		a.mu.RLock()
		defer a.mu.RUnlock()

		if subjectID != "" && a.revokedSubs[subjectID] {
			return true, nil
		}
		if jti != "" && a.revokedJTIs[jti] {
			return true, nil
		}
		return false, nil
	})
}

func (a *MemoryRevocationAdapter) RevokeSubject(ctx context.Context, subjectID string) error {
	return observability.WithSpan(ctx, "memory.revocation.revoke_subject", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.revokedSubs[subjectID] = true
		return nil
	})
}

func (a *MemoryRevocationAdapter) RevokeJTI(ctx context.Context, jti string) error {
	return observability.WithSpan(ctx, "memory.revocation.revoke_jti", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.revokedJTIs[jti] = true
		return nil
	})
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
	return observability.WithSpan(ctx, "memory.access_grant.save", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		key := grant.TenantID + ":" + grant.ID
		a.grants[key] = grant
		return nil
	})
}

func (a *MemoryAccessGrantAdapter) GetGrant(ctx context.Context, tenantID string, id string) (*rbacContracts.AccessGrantRecord, error) {
	return observability.RunTraced(ctx, "memory.access_grant.get", func(ctx context.Context) (*rbacContracts.AccessGrantRecord, error) {
		a.mu.RLock()
		defer a.mu.RUnlock()
		key := tenantID + ":" + id
		g, ok := a.grants[key]
		if !ok {
			return nil, errors.NewNotFoundError("access grant not found")
		}
		return &g, nil
	})
}

func (a *MemoryAccessGrantAdapter) RevokeGrant(ctx context.Context, tenantID string, id string) error {
	return observability.WithSpan(ctx, "memory.access_grant.revoke", func(ctx context.Context) error {
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
	})
}

func (a *MemoryAccessGrantAdapter) ListActiveGrants(ctx context.Context, tenantID string, subjectID string) ([]rbacContracts.AccessGrantRecord, error) {
	return observability.RunTraced(ctx, "memory.access_grant.list", func(ctx context.Context) ([]rbacContracts.AccessGrantRecord, error) {
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
		if res == nil {
			res = []rbacContracts.AccessGrantRecord{}
		}
		return res, nil
	})
}

type MemoryUserBindingAdapter struct {
	mu       sync.RWMutex
	bindings map[string]map[rbacContracts.RoleName]bool
}

func NewMemoryUserBindingAdapter() *MemoryUserBindingAdapter {
	return &MemoryUserBindingAdapter{
		bindings: make(map[string]map[rbacContracts.RoleName]bool),
	}
}

func (a *MemoryUserBindingAdapter) BindUserRole(ctx context.Context, binding rbacContracts.UserBindingRecord) error {
	return observability.WithSpan(ctx, "memory.user_binding.bind", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		key := binding.TenantID + ":" + binding.SubjectID
		if a.bindings[key] == nil {
			a.bindings[key] = make(map[rbacContracts.RoleName]bool)
		}
		a.bindings[key][binding.RoleName] = true
		return nil
	})
}

func (a *MemoryUserBindingAdapter) UnbindUserRole(ctx context.Context, tenantID string, subjectID string, role rbacContracts.RoleName) error {
	return observability.WithSpan(ctx, "memory.user_binding.unbind", func(ctx context.Context) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		key := tenantID + ":" + subjectID
		if a.bindings[key] == nil || !a.bindings[key][role] {
			return errors.NewNotFoundError("user binding not found")
		}
		delete(a.bindings[key], role)
		return nil
	})
}

func (a *MemoryUserBindingAdapter) GetUserRoles(ctx context.Context, tenantID string, subjectID string) ([]rbacContracts.RoleName, error) {
	return observability.RunTraced(ctx, "memory.user_binding.get_roles", func(ctx context.Context) ([]rbacContracts.RoleName, error) {
		a.mu.RLock()
		defer a.mu.RUnlock()
		key := tenantID + ":" + subjectID
		roleMap := a.bindings[key]
		var roles []rbacContracts.RoleName
		for r := range roleMap {
			roles = append(roles, r)
		}
		if roles == nil {
			roles = []rbacContracts.RoleName{}
		}
		return roles, nil
	})
}
