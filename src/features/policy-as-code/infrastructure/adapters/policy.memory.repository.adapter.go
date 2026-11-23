package adapters

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"internal-developer-platform/src/features/policy-as-code/contracts"
	"internal-developer-platform/src/features/policy-as-code/infrastructure/ports"
	"internal-developer-platform/src/shared/errors"
)

type MemoryPolicyRepositoryAdapter struct {
	mu           sync.RWMutex
	policies     map[string]contracts.PolicyRecord
	byPublicID   map[string]string
	byExtID      map[string]string
	exceptions   map[string]contracts.PolicyExceptionRecord
	excByPublic  map[string]string
	violations   map[string]contracts.PolicyViolationRecord
	validFSMFrom map[contracts.PolicyLifecycleState]map[contracts.PolicyLifecycleState]bool
}

var _ ports.PolicyRepositoryPort = (*MemoryPolicyRepositoryAdapter)(nil)

func NewMemoryPolicyRepositoryAdapter() *MemoryPolicyRepositoryAdapter {
	fsm := make(map[contracts.PolicyLifecycleState]map[contracts.PolicyLifecycleState]bool)
	fsm[contracts.LifecycleDraft] = map[contracts.PolicyLifecycleState]bool{
		contracts.LifecycleCIOnly:     true,
		contracts.LifecycleRolledBack: true,
	}
	fsm[contracts.LifecycleCIOnly] = map[contracts.PolicyLifecycleState]bool{
		contracts.LifecycleAudit:      true,
		contracts.LifecycleRolledBack: true,
	}
	fsm[contracts.LifecycleAudit] = map[contracts.PolicyLifecycleState]bool{
		contracts.LifecycleWarn:       true,
		contracts.LifecycleRolledBack: true,
	}
	fsm[contracts.LifecycleWarn] = map[contracts.PolicyLifecycleState]bool{
		contracts.LifecycleDeny:       true,
		contracts.LifecycleRolledBack: true,
	}
	fsm[contracts.LifecycleDeny] = map[contracts.PolicyLifecycleState]bool{
		contracts.LifecycleRolledBack: true,
		contracts.LifecycleRetired:    true,
	}
	fsm[contracts.LifecycleRolledBack] = map[contracts.PolicyLifecycleState]bool{
		contracts.LifecycleDraft: true,
	}

	return &MemoryPolicyRepositoryAdapter{
		policies:     make(map[string]contracts.PolicyRecord),
		byPublicID:   make(map[string]string),
		byExtID:      make(map[string]string),
		exceptions:   make(map[string]contracts.PolicyExceptionRecord),
		excByPublic:  make(map[string]string),
		violations:   make(map[string]contracts.PolicyViolationRecord),
		validFSMFrom: fsm,
	}
}

func (a *MemoryPolicyRepositoryAdapter) SavePolicy(ctx context.Context, policy contracts.PolicyRecord) error {
	if err := policy.Validate(); err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	key := policy.TenantID + ":" + policy.ID
	if _, exists := a.policies[key]; exists {
		return errors.NewConflictError("policy primary key already exists")
	}

	pubKey := policy.PublicID
	if _, exists := a.byPublicID[pubKey]; exists {
		return errors.NewConflictError("policy public_id already exists")
	}

	extKey := policy.TenantID + ":" + string(policy.Source) + ":" + policy.ExternalID
	if _, exists := a.byExtID[extKey]; exists {
		return errors.NewConflictError("policy external_id already exists for tenant and source")
	}

	a.policies[key] = policy
	a.byPublicID[pubKey] = key
	a.byExtID[extKey] = key
	return nil
}

func (a *MemoryPolicyRepositoryAdapter) GetPolicyByID(ctx context.Context, tenantID string, id string) (*contracts.PolicyRecord, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.NewValidationError(ErrEmptyTenantIDMsg)
	}
	if strings.TrimSpace(id) == "" {
		return nil, errors.NewValidationError(ErrEmptyIDMsg)
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	key := tenantID + ":" + id
	policy, exists := a.policies[key]
	if !exists || policy.DeletedAt != nil {
		return nil, errors.NewNotFoundError(ErrPolicyNotFoundMsg)
	}
	return &policy, nil
}

func (a *MemoryPolicyRepositoryAdapter) GetPolicyByPublicID(ctx context.Context, tenantID string, publicID string) (*contracts.PolicyRecord, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.NewValidationError(ErrEmptyTenantIDMsg)
	}
	if strings.TrimSpace(publicID) == "" {
		return nil, errors.NewValidationError(ErrEmptyIDMsg)
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	key, exists := a.byPublicID[publicID]
	if !exists {
		return nil, errors.NewNotFoundError(ErrPolicyNotFoundMsg)
	}
	policy, exists := a.policies[key]
	if !exists || policy.TenantID != tenantID || policy.DeletedAt != nil {
		return nil, errors.NewNotFoundError(ErrPolicyNotFoundMsg)
	}
	return &policy, nil
}

func (a *MemoryPolicyRepositoryAdapter) ListPoliciesKeyset(ctx context.Context, filter contracts.KeysetFilterCriteria) ([]contracts.PolicyRecord, error) {
	if strings.TrimSpace(filter.TenantID) == "" {
		return nil, errors.NewValidationError(ErrEmptyTenantIDMsg)
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	var matching []contracts.PolicyRecord
	for _, p := range a.policies {
		if p.TenantID == filter.TenantID && p.DeletedAt == nil && p.ID > filter.CursorID {
			if filter.LifecycleState != "" && p.LifecycleState != filter.LifecycleState {
				continue
			}
			if filter.EnforcementPoint != "" && p.EnforcementPoint != filter.EnforcementPoint {
				continue
			}
			matching = append(matching, p)
		}
	}

	sort.Slice(matching, func(i, j int) bool {
		return matching[i].ID < matching[j].ID
	})

	limit := filter.PageSize
	if limit <= 0 {
		limit = contracts.DefaultPageSize
	}
	if limit > contracts.MaxPageSize {
		limit = contracts.MaxPageSize
	}

	if len(matching) > limit {
		matching = matching[:limit]
	}
	return matching, nil
}

func (a *MemoryPolicyRepositoryAdapter) ListActiveCELPolicies(ctx context.Context, tenantID string) ([]contracts.PolicyRecord, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.NewValidationError(ErrEmptyTenantIDMsg)
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	var matching []contracts.PolicyRecord
	for _, p := range a.policies {
		if p.TenantID == tenantID && p.EnforcementPoint == contracts.EnforcementPointCEL && p.DeletedAt == nil {
			if p.LifecycleState == contracts.LifecycleAudit || p.LifecycleState == contracts.LifecycleWarn || p.LifecycleState == contracts.LifecycleDeny {
				matching = append(matching, p)
			}
		}
	}

	sort.Slice(matching, func(i, j int) bool {
		return matching[i].ID < matching[j].ID
	})
	return matching, nil
}

func (a *MemoryPolicyRepositoryAdapter) UpdatePolicy(ctx context.Context, policy contracts.PolicyRecord, expectedVersion int64) error {
	if err := policy.Validate(); err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	key := policy.TenantID + ":" + policy.ID
	existing, exists := a.policies[key]
	if !exists || existing.DeletedAt != nil {
		return errors.NewNotFoundError(ErrPolicyNotFoundMsg)
	}
	if existing.Version != expectedVersion {
		return errors.NewConflictError(ErrPolicyConflictMsg)
	}

	policy.Version = existing.Version + 1
	policy.UpdatedAt = time.Now().UTC()
	a.policies[key] = policy
	return nil
}

func (a *MemoryPolicyRepositoryAdapter) UpdatePolicyLifecycle(ctx context.Context, tenantID string, id string, newState contracts.PolicyLifecycleState, expectedVersion int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := tenantID + ":" + id
	existing, exists := a.policies[key]
	if !exists || existing.DeletedAt != nil {
		return errors.NewNotFoundError(ErrPolicyNotFoundMsg)
	}
	if existing.Version != expectedVersion {
		return errors.NewConflictError(ErrPolicyConflictMsg)
	}

	allowedTransitions, ok := a.validFSMFrom[existing.LifecycleState]
	if !ok || !allowedTransitions[newState] {
		return errors.NewValidationError(ErrIllegalStateTransitionMsg)
	}

	existing.LifecycleState = newState
	existing.Version = existing.Version + 1
	existing.UpdatedAt = time.Now().UTC()
	a.policies[key] = existing
	return nil
}

func (a *MemoryPolicyRepositoryAdapter) SoftDeletePolicy(ctx context.Context, tenantID string, id string, expectedVersion int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := tenantID + ":" + id
	existing, exists := a.policies[key]
	if !exists || existing.DeletedAt != nil {
		return errors.NewNotFoundError(ErrPolicyNotFoundMsg)
	}
	if existing.Version != expectedVersion {
		return errors.NewConflictError(ErrPolicyConflictMsg)
	}

	now := time.Now().UTC()
	existing.DeletedAt = &now
	existing.Version = existing.Version + 1
	existing.UpdatedAt = now
	a.policies[key] = existing
	return nil
}

func (a *MemoryPolicyRepositoryAdapter) SaveException(ctx context.Context, exception contracts.PolicyExceptionRecord) error {
	if err := exception.Validate(); err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	policyKey := exception.TenantID + ":" + exception.PolicyID
	policy, policyExists := a.policies[policyKey]
	if !policyExists || policy.DeletedAt != nil {
		return errors.NewNotFoundError("referenced policy does not exist for tenant")
	}

	key := exception.TenantID + ":" + exception.ID
	if _, exists := a.exceptions[key]; exists {
		return errors.NewConflictError("policy exception primary key already exists")
	}

	pubKey := exception.PublicID
	if _, exists := a.excByPublic[pubKey]; exists {
		return errors.NewConflictError("policy exception public_id already exists")
	}

	a.exceptions[key] = exception
	a.excByPublic[pubKey] = key
	return nil
}

func (a *MemoryPolicyRepositoryAdapter) GetExceptionByID(ctx context.Context, tenantID string, id string) (*contracts.PolicyExceptionRecord, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.NewValidationError(ErrEmptyTenantIDMsg)
	}
	if strings.TrimSpace(id) == "" {
		return nil, errors.NewValidationError(ErrEmptyIDMsg)
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	key := tenantID + ":" + id
	exc, exists := a.exceptions[key]
	if !exists || exc.DeletedAt != nil {
		return nil, errors.NewNotFoundError(ErrExceptionNotFoundMsg)
	}
	return &exc, nil
}

func (a *MemoryPolicyRepositoryAdapter) GetActiveExceptionsForWorkload(ctx context.Context, tenantID string, namespace string, workloadID string) ([]contracts.PolicyExceptionRecord, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.NewValidationError(ErrEmptyTenantIDMsg)
	}
	if strings.TrimSpace(namespace) == "" {
		return nil, errors.NewValidationError(ErrEmptyNamespaceMsg)
	}
	if strings.TrimSpace(workloadID) == "" {
		return nil, errors.NewValidationError(ErrEmptyWorkloadMsg)
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	now := time.Now().UTC()
	var matching []contracts.PolicyExceptionRecord
	for _, exc := range a.exceptions {
		if exc.TenantID == tenantID && exc.Namespace == namespace && exc.WorkloadID == workloadID && exc.DeletedAt == nil {
			if exc.Status == contracts.ExceptionStatusActive && exc.ExpiresAt.After(now) {
				matching = append(matching, exc)
			}
		}
	}
	return matching, nil
}

func (a *MemoryPolicyRepositoryAdapter) UpdateExceptionStatus(ctx context.Context, tenantID string, id string, newStatus contracts.ExceptionStatus, expectedVersion int64) error {
	if strings.TrimSpace(tenantID) == "" {
		return errors.NewValidationError(ErrEmptyTenantIDMsg)
	}
	if strings.TrimSpace(id) == "" {
		return errors.NewValidationError(ErrEmptyIDMsg)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	key := tenantID + ":" + id
	existing, exists := a.exceptions[key]
	if !exists || existing.DeletedAt != nil {
		return errors.NewNotFoundError(ErrExceptionNotFoundMsg)
	}
	if existing.Version != expectedVersion {
		return errors.NewConflictError(ErrExceptionConflictMsg)
	}

	existing.Status = newStatus
	existing.Version = existing.Version + 1
	existing.UpdatedAt = time.Now().UTC()
	a.exceptions[key] = existing
	return nil
}

func (a *MemoryPolicyRepositoryAdapter) ReconcileExpiredExceptions(ctx context.Context, tenantID string, now time.Time) (int, error) {
	if strings.TrimSpace(tenantID) == "" {
		return 0, errors.NewValidationError(ErrEmptyTenantIDMsg)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	reconciledCount := 0
	for key, exc := range a.exceptions {
		if (exc.TenantID == tenantID) && (exc.Status == contracts.ExceptionStatusActive) && (exc.DeletedAt == nil) && (!exc.ExpiresAt.After(now)) {
			exc.Status = contracts.ExceptionStatusExpired
			exc.Version = exc.Version + 1
			exc.UpdatedAt = now
			a.exceptions[key] = exc
			reconciledCount++
		}
	}
	return reconciledCount, nil
}

func (a *MemoryPolicyRepositoryAdapter) SaveViolation(ctx context.Context, v contracts.PolicyViolationRecord) error {
	if err := v.Validate(); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	key := v.TenantID + ":" + v.ID
	a.violations[key] = v
	return nil
}

func (a *MemoryPolicyRepositoryAdapter) ListActiveViolationsByPolicy(ctx context.Context, tenantID string, policyID string) ([]contracts.PolicyViolationRecord, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var result []contracts.PolicyViolationRecord
	for _, v := range a.violations {
		if (v.TenantID == tenantID) && (v.PolicyID == policyID) && (v.Status == contracts.ViolationStatusActive) {
			result = append(result, v)
		}
	}
	return result, nil
}

func (a *MemoryPolicyRepositoryAdapter) CountActiveViolationsByPolicy(ctx context.Context, tenantID string, policyID string) (int, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	count := 0
	for _, v := range a.violations {
		if (v.TenantID == tenantID) && (v.PolicyID == policyID) && (v.Status == contracts.ViolationStatusActive) {
			count++
		}
	}
	return count, nil
}

func (a *MemoryPolicyRepositoryAdapter) ResolveViolation(ctx context.Context, tenantID string, violationID string, expectedVersion int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	key := tenantID + ":" + violationID
	v, exists := a.violations[key]
	if !exists {
		return errors.NewNotFoundError("violation not found")
	}
	if v.Version != expectedVersion {
		return errors.NewConflictError("violation version conflict")
	}
	v.Status = contracts.ViolationStatusResolved
	v.Version++
	v.UpdatedAt = time.Now().UTC()
	a.violations[key] = v
	return nil
}
