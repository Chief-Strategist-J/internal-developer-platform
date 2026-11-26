/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC SERVICE (feature-files.md §4 COMPLIANT)

1. Public Functions (grouped by shape):
  - S1 Command:
    * RequestJITAccess(ctx, req) -> (*AccessGrantRecord, error) [deadline -> validate -> UnitOfWork(persistGrant + auditOutbox)]
    * RevokeJITAccess(ctx, tenantID, grantID) -> error [deadline -> UnitOfWork(loadGrant -> machine.TransitionGrantStatus -> persistRevokeGrant + auditOutbox)]
    * BindUserRole(ctx, binding) -> error [deadline -> UnitOfWork(persistUserBinding + auditOutbox)]
    * UnbindUserRole(ctx, tenantID, subjectID, role) -> error [deadline -> invalidateCache -> UnitOfWork(removeUserBinding + auditOutbox)]
    * RevokeSubject(ctx, subjectID) -> error [deadline -> invalidateCache -> revocationStore.RevokeSubject]
    * RevokeJTI(ctx, jti) -> error [deadline -> revocationStore.RevokeJTI]
  - S2 Query:
    * AuthenticateToken(ctx, rawToken) -> (*Subject, error) [deadline -> verifyToken -> fail-closed revocation check]
    * Authorize(ctx, subject, targetTenant, resource, action) -> error
    * AuthenticateAndAuthorize(ctx, rawToken, targetTenant, resource, action) -> (*Subject, error)
    * AuthorizeWithContext(ctx, req) -> (EvaluationResult, error) [deadline -> permitCache -> rules.EvaluateAuthorization -> auditOutbox / denyFailoverLog]
    * ListActiveGrants(ctx, tenantID, subjectID) -> ([]AccessGrantRecord, error)
    * ListActiveGrantsPage(ctx, tenantID, subjectID, pageSize, cursor) -> (AccessGrantPage, error)
    * GetUserRoles(ctx, tenantID, subjectID) -> ([]RoleName, error)
    * ValidateRoleHierarchy(ctx, edges) -> error
    * VerifyImpersonation(ctx, callerMTLS, hasImpersonateGrant, targetProtectedAdmin) -> error
    * VerifyDelegationScope(ctx, isTenantDelegation, tokenTenantID, targetTenantID) -> error
    * EvaluateTaskLease(ctx, req) -> (TaskExecutionLeaseResult, error)
    * EvaluateAsyncJobDrift(ctx, subjectID, jti, queueLatency, oidcIntrospect) -> (AsyncJobRevocationResult, error)
    * EvaluateSplitBrain(ctx, globalAuthorityReachable, isRoleMutation, snapshotAge) -> (RegionalSplitBrainResult, error)
    * EvaluateStreamingHeartbeat(expiresAt, sessionStart) -> StreamingHeartbeatResult

2. Private Functions (grouped by purpose):
  - loading: loadGrant, findActiveGrants
  - lifecycle: transitionGrantForRevoke
  - policy: validateJITPolicy
  - computation: buildInitialGrantRecord, clampPageSize, buildPermitCacheKey, shouldBypassPermitCache
  - persistence: persistGrant, persistRevokeGrant, persistUserBinding, removeUserBinding
  - messaging: dispatchAuditDecision, dispatchLifecycleAudit, writeDenyFailoverLog
  - transaction: runInUnitOfWork

3. Invariants:
  - Zero mid-function inline comments.
  - Constructor injection via RBACServiceDeps.
*/
package service

import (
	"container/list"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/features/role-based-access-control/domain/machines"
	"internal-developer-platform/src/features/role-based-access-control/domain/rules"
	"internal-developer-platform/src/features/role-based-access-control/infrastructure/ports"
	"internal-developer-platform/src/infra/clients/resilience"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/errors"
)

const (
	rbacSpanPrefix      = "rbac.service"
	permitCacheTTL      = 30 * time.Second
	permitCacheCapacity = 1024
	rbacMaxPageSize     = 100
	rbacDefaultPageSize = 25
	rbacDeadlineMargin  = 10 * time.Millisecond
)

type RBACServiceDeps struct {
	TokenVerifier        ports.TokenVerifierPort
	RevocationStore      ports.RevocationStorePort
	GrantRepo            ports.AccessGrantRepositoryPort
	BindingRepo          ports.UserBindingRepositoryPort
	AuditOutbox          ports.RBACAuditOutboxPort
	UnitOfWork           ports.UnitOfWorkPort
	TicketVerifier       ports.BreakGlassTicketVerifierPort
	SIEMAlertEmitter     ports.SIEMAlertEmitterPort
	RoleDefRepo          ports.RoleDefinitionRepositoryPort
	OIDCIntrospector     ports.OIDCIntrospectorPort
	ExclusiveClaimRepo   ports.ExclusiveClaimPort
	FederationHMACSecret string
	TaskLeaseHMACSecret  string
	DevStagingMaxQuota   int
	AllowedStartHourUTC  int
	AllowedEndHourUTC    int
}

type permitCacheEntry struct {
	key       string
	subjectID string
	result    rbacContracts.EvaluationResult
	expiresAt time.Time
}

type permitLRUCache struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	items    map[string]*list.Element
	order    *list.List
}

func newPermitLRUCache(capacity int, ttl time.Duration) *permitLRUCache {
	return &permitLRUCache{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

func (c *permitLRUCache) get(key string, now time.Time) (rbacContracts.EvaluationResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.items[key]
	if !ok {
		return rbacContracts.EvaluationResult{}, false
	}
	entry := elem.Value.(*permitCacheEntry)
	if !now.Before(entry.expiresAt) {
		c.order.Remove(elem)
		delete(c.items, key)
		return rbacContracts.EvaluationResult{}, false
	}
	c.order.MoveToFront(elem)
	return entry.result, true
}

func (c *permitLRUCache) set(key string, subjectID string, result rbacContracts.EvaluationResult, now time.Time) {
	if !result.Granted || result.ElevatedPermit {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[key]; ok {
		entry := elem.Value.(*permitCacheEntry)
		entry.result = result
		entry.expiresAt = now.Add(c.ttl)
		c.order.MoveToFront(elem)
		return
	}
	if c.order.Len() >= c.capacity {
		oldest := c.order.Back()
		if oldest != nil {
			c.order.Remove(oldest)
			delete(c.items, oldest.Value.(*permitCacheEntry).key)
		}
	}
	entry := &permitCacheEntry{
		key:       key,
		subjectID: subjectID,
		result:    result,
		expiresAt: now.Add(c.ttl),
	}
	elem := c.order.PushFront(entry)
	c.items[key] = elem
}

func (c *permitLRUCache) invalidateSubject(subjectID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, elem := range c.items {
		if elem.Value.(*permitCacheEntry).subjectID == subjectID {
			c.order.Remove(elem)
			delete(c.items, k)
		}
	}
}

type RBACService struct {
	tokenVerifier        ports.TokenVerifierPort
	revocationStore      ports.RevocationStorePort
	grantRepo            ports.AccessGrantRepositoryPort
	bindingRepo          ports.UserBindingRepositoryPort
	auditOutbox          ports.RBACAuditOutboxPort
	unitOfWork           ports.UnitOfWorkPort
	ticketVerifier       ports.BreakGlassTicketVerifierPort
	siemAlertEmitter     ports.SIEMAlertEmitterPort
	roleDefRepo          ports.RoleDefinitionRepositoryPort
	oidcIntrospector     ports.OIDCIntrospectorPort
	exclusiveClaimRepo   ports.ExclusiveClaimPort
	sidecarPDP           ports.SidecarPDPPort
	federationHMACSecret string
	taskLeaseHMACSecret  string
	devStagingMaxQuota   int
	allowedStartHourUTC  int
	allowedEndHourUTC    int
	permitCache          *permitLRUCache
	streamingSessions    sync.Map
}

type activeStreamingSession struct {
	conn      io.Closer
	expiresAt time.Time
	startedAt time.Time
}

func NewRBACServiceWithDeps(deps RBACServiceDeps) *RBACService {
	return &RBACService{
		tokenVerifier:        deps.TokenVerifier,
		revocationStore:      deps.RevocationStore,
		grantRepo:            deps.GrantRepo,
		bindingRepo:          deps.BindingRepo,
		auditOutbox:          deps.AuditOutbox,
		unitOfWork:           deps.UnitOfWork,
		ticketVerifier:       deps.TicketVerifier,
		siemAlertEmitter:     deps.SIEMAlertEmitter,
		roleDefRepo:          deps.RoleDefRepo,
		oidcIntrospector:     deps.OIDCIntrospector,
		exclusiveClaimRepo:   deps.ExclusiveClaimRepo,
		federationHMACSecret: deps.FederationHMACSecret,
		taskLeaseHMACSecret:  deps.TaskLeaseHMACSecret,
		devStagingMaxQuota:   deps.DevStagingMaxQuota,
		allowedStartHourUTC:  deps.AllowedStartHourUTC,
		allowedEndHourUTC:    deps.AllowedEndHourUTC,
		permitCache:          newPermitLRUCache(permitCacheCapacity, permitCacheTTL),
	}
}

func NewRBACService(verifier ports.TokenVerifierPort, revocation ports.RevocationStorePort, grantRepo ...ports.AccessGrantRepositoryPort) *RBACService {
	deps := RBACServiceDeps{
		TokenVerifier:   verifier,
		RevocationStore: revocation,
	}
	if len(grantRepo) > 0 && grantRepo[0] != nil {
		deps.GrantRepo = grantRepo[0]
	}
	return NewRBACServiceWithDeps(deps)
}

func (s *RBACService) SetGrantRepository(repo ports.AccessGrantRepositoryPort) {
	s.grantRepo = repo
}

func (s *RBACService) SetUserBindingRepository(repo ports.UserBindingRepositoryPort) {
	s.bindingRepo = repo
}

func (s *RBACService) SetAuditOutbox(outbox ports.RBACAuditOutboxPort) {
	s.auditOutbox = outbox
}

func (s *RBACService) SetUnitOfWork(uow ports.UnitOfWorkPort) {
	s.unitOfWork = uow
}

func (s *RBACService) SetSidecarPDP(pdp ports.SidecarPDPPort) {
	s.sidecarPDP = pdp
}

func (s *RBACService) EvaluateWithSidecarPDP(ctx context.Context, req rbacContracts.EvaluationRequest) (rbacContracts.EvaluationResult, error) {
	now := time.Now().UTC()
	var sidecarResult *rbacContracts.SidecarDecisionResult
	if s.sidecarPDP != nil {
		res, err := s.sidecarPDP.EvaluateSidecar(ctx, rbacContracts.SidecarDecisionRequest{
			SubjectID:   req.Subject.ID,
			TenantID:    req.TargetTenantID,
			Resource:    req.Resource,
			Action:      req.Action,
			Environment: req.TargetEnvironment,
		})
		if err == nil {
			sidecarResult = &res
		}
	}
	return rules.EvaluateSidecarPDPFallback(sidecarResult, req, now), nil
}

func (s *RBACService) AnalyzeLeastPrivilege(roles map[rbacContracts.RoleName][]rbacContracts.Permission, usage []rbacContracts.PermissionUsageRecord, staleWindow time.Duration) rbacContracts.LeastPrivilegeReport {
	return rules.AnalyzeLeastPrivilege(roles, usage, staleWindow, time.Now().UTC())
}

func (s *RBACService) AuthenticateToken(ctx context.Context, rawToken string) (*rbacContracts.Subject, error) {
	return observability.RunTraced(ctx, rbacSpanPrefix+".authenticate_token", func(ctx context.Context) (*rbacContracts.Subject, error) {
		if err := resilience.ValidateDeadlineBudget(ctx, rbacDeadlineMargin, 0); err != nil {
			return nil, err
		}
		subject, err := s.tokenVerifier.VerifyToken(ctx, rawToken)
		if err != nil {
			return nil, err
		}
		if s.revocationStore != nil {
			revoked, err := s.revocationStore.IsRevoked(ctx, subject.ID, subject.JTI)
			if err != nil {
				return nil, errors.NewUnauthorizedError(constants.ErrRevocationStoreUnavailable)
			}
			if revoked {
				return nil, errors.NewUnauthorizedError("subject or token has been revoked")
			}
		}
		return subject, nil
	})
}

func (s *RBACService) Authorize(ctx context.Context, subject rbacContracts.Subject, targetTenant string, resource rbacContracts.Resource, action rbacContracts.Action) error {
	return observability.WithSpan(ctx, rbacSpanPrefix+".authorize", func(ctx context.Context) error {
		if err := resilience.ValidateDeadlineBudget(ctx, rbacDeadlineMargin, 0); err != nil {
			return err
		}
		req := rbacContracts.EvaluationRequest{
			Subject:        subject,
			TargetTenantID: targetTenant,
			Resource:       resource,
			Action:         action,
		}
		_, err := s.AuthorizeWithContext(ctx, req)
		return err
	}, attribute.String("subject.id", subject.ID), attribute.String("target_tenant", targetTenant), attribute.String("resource", string(resource)), attribute.String("action", string(action)))
}

func (s *RBACService) AuthenticateAndAuthorize(ctx context.Context, rawToken string, targetTenant string, resource rbacContracts.Resource, action rbacContracts.Action) (*rbacContracts.Subject, error) {
	return observability.RunTraced(ctx, rbacSpanPrefix+".authenticate_and_authorize", func(ctx context.Context) (*rbacContracts.Subject, error) {
		if err := resilience.ValidateDeadlineBudget(ctx, rbacDeadlineMargin, 0); err != nil {
			return nil, err
		}
		subject, err := s.AuthenticateToken(ctx, rawToken)
		if err != nil {
			return nil, err
		}
		if err := s.Authorize(ctx, *subject, targetTenant, resource, action); err != nil {
			return nil, err
		}
		return subject, nil
	}, attribute.String("target_tenant", targetTenant), attribute.String("resource", string(resource)), attribute.String("action", string(action)))
}

func (s *RBACService) IssueToken(subject rbacContracts.Subject) (string, error) {
	ctx := context.Background()
	return observability.RunTraced(ctx, rbacSpanPrefix+".issue_token", func(ctx context.Context) (string, error) {
		resolvedSubject := subject
		if len(resolvedSubject.Roles) == 0 && s.bindingRepo != nil {
			roles, err := s.bindingRepo.GetUserRoles(ctx, resolvedSubject.TenantID, resolvedSubject.ID)
			if err == nil && len(roles) > 0 {
				resolvedSubject.Roles = roles
			}
		}
		return s.tokenVerifier.IssueToken(resolvedSubject)
	}, attribute.String("subject.id", subject.ID), attribute.String("tenant_id", subject.TenantID))
}

func (s *RBACService) RevokeSubject(ctx context.Context, subjectID string) error {
	return observability.WithSpan(ctx, rbacSpanPrefix+".revoke_subject", func(ctx context.Context) error {
		if err := resilience.ValidateDeadlineBudget(ctx, rbacDeadlineMargin, 0); err != nil {
			return err
		}
		if s.permitCache != nil {
			s.permitCache.invalidateSubject(subjectID)
		}
		if s.revocationStore != nil {
			return s.revocationStore.RevokeSubject(ctx, subjectID)
		}
		return nil
	}, attribute.String("subject.id", subjectID))
}

func (s *RBACService) RevokeJTI(ctx context.Context, jti string) error {
	return observability.WithSpan(ctx, rbacSpanPrefix+".revoke_jti", func(ctx context.Context) error {
		if err := resilience.ValidateDeadlineBudget(ctx, rbacDeadlineMargin, 0); err != nil {
			return err
		}
		if jti == "" {
			return errors.NewValidationError("jti is required")
		}
		if s.revocationStore != nil {
			return s.revocationStore.RevokeJTI(ctx, jti)
		}
		return nil
	}, attribute.String("jti", jti))
}

func (s *RBACService) InvalidateLocalCache(subjectID string) {
	if s.permitCache != nil && subjectID != "" {
		s.permitCache.invalidateSubject(subjectID)
	}
}

func (s *RBACService) RequestJITAccess(ctx context.Context, req rbacContracts.AccessGrantRequest) (*rbacContracts.AccessGrantRecord, error) {
	return observability.RunTraced(ctx, rbacSpanPrefix+".request_jit_access", func(ctx context.Context) (*rbacContracts.AccessGrantRecord, error) {
		if err := resilience.ValidateDeadlineBudget(ctx, rbacDeadlineMargin, 0); err != nil {
			return nil, err
		}
		if err := s.validateJITPolicy(req); err != nil {
			return nil, err
		}
		record := buildInitialGrantRecord(req, time.Now().UTC())
		err := s.runInUnitOfWork(ctx, func(txCtx context.Context) error {
			if err := s.persistGrant(txCtx, record); err != nil {
				return err
			}
			return s.dispatchLifecycleAudit(txCtx, record.TenantID, record.SubjectID, rbacContracts.ResourceAuth, rbacContracts.ActionWrite, "PERMIT", "jit access grant created")
		})
		if err != nil {
			return nil, err
		}
		return &record, nil
	}, attribute.String("tenant_id", req.TenantID), attribute.String("subject.id", req.SubjectID), attribute.String("role", req.Role))
}

func (s *RBACService) RevokeJITAccess(ctx context.Context, tenantID, grantID string) error {
	return observability.WithSpan(ctx, rbacSpanPrefix+".revoke_jit_access", func(ctx context.Context) error {
		if err := resilience.ValidateDeadlineBudget(ctx, rbacDeadlineMargin, 0); err != nil {
			return err
		}
		if tenantID == "" || grantID == "" {
			return errors.NewValidationError("tenantId and grantId are required")
		}
		return s.runInUnitOfWork(ctx, func(txCtx context.Context) error {
			grant, err := s.loadGrant(txCtx, tenantID, grantID)
			if err != nil {
				return err
			}
			if grant != nil {
				if _, err := transitionGrantForRevoke(grant.Status); err != nil {
					return err
				}
			}
			if err := s.persistRevokeGrant(txCtx, tenantID, grantID); err != nil {
				return err
			}
			subjectID := grantID
			if grant != nil && grant.SubjectID != "" {
				subjectID = grant.SubjectID
			}
			return s.dispatchLifecycleAudit(txCtx, tenantID, subjectID, rbacContracts.ResourceAuth, rbacContracts.ActionDelete, "PERMIT", "jit access grant revoked")
		})
	}, attribute.String("tenant_id", tenantID), attribute.String("grant_id", grantID))
}

func (s *RBACService) ListActiveGrants(ctx context.Context, tenantID, subjectID string) ([]rbacContracts.AccessGrantRecord, error) {
	return observability.RunTraced(ctx, rbacSpanPrefix+".list_active_grants", func(ctx context.Context) ([]rbacContracts.AccessGrantRecord, error) {
		if err := resilience.ValidateDeadlineBudget(ctx, rbacDeadlineMargin, 0); err != nil {
			return nil, err
		}
		return s.findActiveGrants(ctx, tenantID, subjectID)
	}, attribute.String("tenant_id", tenantID), attribute.String("subject.id", subjectID))
}

func (s *RBACService) ListActiveGrantsPage(ctx context.Context, tenantID string, subjectID string, pageSize int, cursor string) (rbacContracts.AccessGrantPage, error) {
	return observability.RunTraced(ctx, rbacSpanPrefix+".list_active_grants_page", func(ctx context.Context) (rbacContracts.AccessGrantPage, error) {
		if err := resilience.ValidateDeadlineBudget(ctx, rbacDeadlineMargin, 0); err != nil {
			return rbacContracts.AccessGrantPage{}, err
		}
		all, err := s.findActiveGrants(ctx, tenantID, subjectID)
		if err != nil {
			return rbacContracts.AccessGrantPage{}, err
		}
		limit := clampPageSize(pageSize, rbacMaxPageSize)
		offset := decodeCursor(cursor)
		if offset >= len(all) {
			return rbacContracts.AccessGrantPage{Items: []rbacContracts.AccessGrantRecord{}, HasMore: false}, nil
		}
		end := offset + limit
		hasMore := end < len(all)
		if end > len(all) {
			end = len(all)
		}
		nextCursor := ""
		if hasMore {
			nextCursor = strconv.Itoa(end)
		}
		return rbacContracts.AccessGrantPage{
			Items:      all[offset:end],
			NextCursor: nextCursor,
			HasMore:    hasMore,
		}, nil
	}, attribute.String("tenant_id", tenantID), attribute.String("subject.id", subjectID))
}

func (s *RBACService) BindUserRole(ctx context.Context, binding rbacContracts.UserBindingRecord) error {
	return observability.WithSpan(ctx, rbacSpanPrefix+".bind_user_role", func(ctx context.Context) error {
		if err := resilience.ValidateDeadlineBudget(ctx, rbacDeadlineMargin, 0); err != nil {
			return err
		}
		if binding.TenantID == "" || binding.SubjectID == "" || binding.RoleName == "" {
			return errors.NewValidationError("tenantId, subjectId, and roleName are required")
		}
		return s.runInUnitOfWork(ctx, func(txCtx context.Context) error {
			if err := s.persistUserBinding(txCtx, binding); err != nil {
				return err
			}
			return s.dispatchLifecycleAudit(txCtx, binding.TenantID, binding.SubjectID, rbacContracts.ResourceAuth, rbacContracts.ActionWrite, "PERMIT", "user role bound")
		})
	}, attribute.String("tenant_id", binding.TenantID), attribute.String("subject_id", binding.SubjectID), attribute.String("role", string(binding.RoleName)))
}

func (s *RBACService) UnbindUserRole(ctx context.Context, tenantID string, subjectID string, role rbacContracts.RoleName) error {
	return observability.WithSpan(ctx, rbacSpanPrefix+".unbind_user_role", func(ctx context.Context) error {
		if err := resilience.ValidateDeadlineBudget(ctx, rbacDeadlineMargin, 0); err != nil {
			return err
		}
		if tenantID == "" || subjectID == "" || role == "" {
			return errors.NewValidationError("tenantId, subjectId, and roleName are required")
		}
		if s.permitCache != nil {
			s.permitCache.invalidateSubject(subjectID)
		}
		return s.runInUnitOfWork(ctx, func(txCtx context.Context) error {
			if err := s.removeUserBinding(txCtx, tenantID, subjectID, role); err != nil {
				return err
			}
			return s.dispatchLifecycleAudit(txCtx, tenantID, subjectID, rbacContracts.ResourceAuth, rbacContracts.ActionDelete, "PERMIT", "user role unbound")
		})
	}, attribute.String("tenant_id", tenantID), attribute.String("subject_id", subjectID), attribute.String("role", string(role)))
}

func (s *RBACService) GetUserRoles(ctx context.Context, tenantID string, subjectID string) ([]rbacContracts.RoleName, error) {
	return observability.RunTraced(ctx, rbacSpanPrefix+".get_user_roles", func(ctx context.Context) ([]rbacContracts.RoleName, error) {
		if err := resilience.ValidateDeadlineBudget(ctx, rbacDeadlineMargin, 0); err != nil {
			return nil, err
		}
		if tenantID == "" || subjectID == "" {
			return nil, errors.NewValidationError("tenantId and subjectId are required")
		}
		if s.bindingRepo != nil {
			return s.bindingRepo.GetUserRoles(ctx, tenantID, subjectID)
		}
		return []rbacContracts.RoleName{}, nil
	}, attribute.String("tenant_id", tenantID), attribute.String("subject_id", subjectID))
}

func (s *RBACService) AuthorizeWithContext(ctx context.Context, req rbacContracts.EvaluationRequest) (rbacContracts.EvaluationResult, error) {
	ctx = observability.WithTenantID(ctx, req.TargetTenantID)
	return observability.RunTraced(ctx, rbacSpanPrefix+".authorize_with_context", func(ctx context.Context) (rbacContracts.EvaluationResult, error) {
		if err := resilience.ValidateDeadlineBudget(ctx, rbacDeadlineMargin, 0); err != nil {
			return rbacContracts.EvaluationResult{}, err
		}
		now := time.Now()
		req = s.enrichEvaluationRequest(ctx, req)
		bypassCache := shouldBypassPermitCache(req)
		cacheKey := buildPermitCacheKey(req)
		if !bypassCache && s.permitCache != nil {
			if cached, ok := s.permitCache.get(cacheKey, now); ok {
				_ = s.dispatchAuditDecision(ctx, req, "PERMIT", cached)
				return cached, nil
			}
		}
		decision := rules.EvaluateAuthorization(req, now)
		if !decision.Granted {
			observability.GetLogger().Warn(ctx, "rbac authorization denied",
				observability.String("subject_id", req.Subject.ID),
				observability.String("resource", string(req.Resource)),
				observability.String("action", string(req.Action)),
				observability.String("violation_code", decision.ViolationCode),
				observability.String("reason", decision.Reason),
			)
			if err := s.dispatchAuditDecision(ctx, req, "DENY", decision); err != nil {
				s.writeDenyFailoverLog(ctx, req, decision, err)
			}
			return decision, errors.NewForbiddenError(decision.Reason)
		}
		if decision.ElevatedPermit {
			if err := s.dispatchAuditDecision(ctx, req, "ELEVATED_PERMIT", decision); err != nil {
				observability.GetLogger().Error(ctx, "rbac break-glass audit guarantee failed",
					observability.String("subject_id", req.Subject.ID),
					observability.String("resource", string(req.Resource)),
					observability.String("error", err.Error()),
				)
				return rbacContracts.EvaluationResult{
					Granted:       false,
					Reason:        "cannot grant break-glass access without audit guarantee",
					ViolationCode: "ERR_AUDIT_UNAVAILABLE",
				}, errors.NewForbiddenError("cannot grant break-glass access without audit guarantee")
			}
			s.emitBreakGlassSIEMAlert(ctx, req, decision)
			return decision, nil
		}
		if !bypassCache && s.permitCache != nil {
			s.permitCache.set(cacheKey, req.Subject.ID, decision, now)
		}
		_ = s.dispatchAuditDecision(ctx, req, "PERMIT", decision)
		return decision, nil
	}, attribute.String("subject.id", req.Subject.ID), attribute.String("target_tenant", req.TargetTenantID), attribute.String("resource", string(req.Resource)), attribute.String("action", string(req.Action)))
}

func (s *RBACService) PermitCacheSize() int {
	if s.permitCache == nil {
		return 0
	}
	s.permitCache.mu.Lock()
	defer s.permitCache.mu.Unlock()
	return s.permitCache.order.Len()
}

func (s *RBACService) ValidateRoleHierarchy(ctx context.Context, edges map[string][]string) error {
	return observability.WithSpan(ctx, rbacSpanPrefix+".validate_role_hierarchy", func(ctx context.Context) error {
		if err := resilience.ValidateDeadlineBudget(ctx, rbacDeadlineMargin, 0); err != nil {
			return err
		}
		if err := rules.ValidateRoleHierarchyDAG(edges); err != nil {
			observability.GetLogger().Warn(ctx, "rbac role hierarchy validation failed",
				observability.Int("edge_count", len(edges)),
				observability.String("error", err.Error()),
			)
			return err
		}
		return nil
	}, attribute.Int("rbac.edge_count", len(edges)))
}

func (s *RBACService) VerifyImpersonation(ctx context.Context, callerMTLS bool, hasImpersonateGrant bool, targetProtectedAdmin bool) error {
	return observability.WithSpan(ctx, rbacSpanPrefix+".verify_impersonation", func(ctx context.Context) error {
		if err := rules.EvaluateImpersonation(callerMTLS, hasImpersonateGrant, targetProtectedAdmin); err != nil {
			observability.GetLogger().Warn(ctx, "rbac impersonation verification failed",
				observability.Bool("caller_mtls", callerMTLS),
				observability.Bool("has_impersonate_grant", hasImpersonateGrant),
				observability.Bool("target_protected_admin", targetProtectedAdmin),
				observability.String("error", err.Error()),
			)
			return err
		}
		return nil
	}, attribute.Bool("caller.mtls", callerMTLS), attribute.Bool("impersonate.grant", hasImpersonateGrant), attribute.Bool("target.protected_admin", targetProtectedAdmin))
}

func (s *RBACService) VerifyDelegationScope(ctx context.Context, isTenantDelegation bool, tokenTenantID string, targetTenantID string) error {
	ctx = observability.WithTenantID(ctx, targetTenantID)
	return observability.WithSpan(ctx, rbacSpanPrefix+".verify_delegation_scope", func(ctx context.Context) error {
		if err := rules.EvaluateDelegationScope(isTenantDelegation, tokenTenantID, targetTenantID); err != nil {
			observability.GetLogger().Warn(ctx, "rbac delegation scope verification failed",
				observability.Bool("is_tenant_delegation", isTenantDelegation),
				observability.String("token_tenant_id", tokenTenantID),
				observability.String("target_tenant_id", targetTenantID),
				observability.String("error", err.Error()),
			)
			return err
		}
		return nil
	}, attribute.Bool("delegation.tenant_scoped", isTenantDelegation), attribute.String("token.tenant_id", tokenTenantID), attribute.String("target.tenant_id", targetTenantID))
}

func (s *RBACService) EvaluateTaskLease(ctx context.Context, req rbacContracts.TaskExecutionLeaseRequest) (rbacContracts.TaskExecutionLeaseResult, error) {
	return observability.RunTraced(ctx, rbacSpanPrefix+".evaluate_task_lease", func(ctx context.Context) (rbacContracts.TaskExecutionLeaseResult, error) {
		res := rules.EvaluateTaskExecutionLease(req)
		if !res.Granted {
			return res, errors.NewUnauthorizedError(res.Reason)
		}
		return res, nil
	})
}

func (s *RBACService) EvaluateAsyncJobDrift(ctx context.Context, subjectID string, jti string, queueLatency time.Duration, oidcIntrospect func(ctx context.Context) (bool, error)) (rbacContracts.AsyncJobRevocationResult, error) {
	return observability.RunTraced(ctx, rbacSpanPrefix+".evaluate_async_job_drift", func(ctx context.Context) (rbacContracts.AsyncJobRevocationResult, error) {
		revoked := false
		if s.revocationStore != nil {
			r, err := s.revocationStore.IsRevoked(ctx, subjectID, jti)
			if err != nil {
				return rbacContracts.AsyncJobRevocationResult{Granted: false, Directive: "DISCARD", Reason: constants.ErrRevocationStoreUnavailable, ViolationCode: "ERR_REVOCATION_UNAVAILABLE"}, errors.NewUnauthorizedError(constants.ErrRevocationStoreUnavailable)
			}
			revoked = r
		}
		res := rules.EvaluateAsyncJobRevocationDrift(ctx, revoked, queueLatency, oidcIntrospect)
		if !res.Granted {
			return res, errors.NewForbiddenError(res.Reason)
		}
		return res, nil
	}, attribute.String("subject.id", subjectID))
}

func (s *RBACService) EvaluateSplitBrain(ctx context.Context, globalAuthorityReachable bool, isRoleMutation bool, snapshotAge time.Duration) (rbacContracts.RegionalSplitBrainResult, error) {
	return observability.RunTraced(ctx, rbacSpanPrefix+".evaluate_split_brain", func(ctx context.Context) (rbacContracts.RegionalSplitBrainResult, error) {
		res := rules.EvaluateRegionalSplitBrain(globalAuthorityReachable, isRoleMutation, snapshotAge)
		if !res.Granted {
			return res, errors.NewForbiddenError(res.Reason)
		}
		return res, nil
	}, attribute.Bool("is_role_mutation", isRoleMutation))
}

func (s *RBACService) EvaluateStreamingHeartbeat(expiresAt time.Time, sessionStart time.Time) rbacContracts.StreamingHeartbeatResult {
	return rules.EvaluateStreamingSessionHeartbeat(expiresAt, sessionStart, time.Now().UTC())
}

func (s *RBACService) runInUnitOfWork(ctx context.Context, fn func(txCtx context.Context) error) error {
	if s.unitOfWork != nil {
		return s.unitOfWork.Run(ctx, fn)
	}
	return fn(ctx)
}

func (s *RBACService) enrichEvaluationRequest(ctx context.Context, req rbacContracts.EvaluationRequest) rbacContracts.EvaluationRequest {
	if req.FederationHMACSecret == "" {
		req.FederationHMACSecret = s.federationHMACSecret
	}
	if req.MaxTenantQuota == 0 {
		req.MaxTenantQuota = s.devStagingMaxQuota
	}
	if req.AllowedEndHourUTC == 0 {
		req.AllowedStartHourUTC = s.allowedStartHourUTC
		req.AllowedEndHourUTC = s.allowedEndHourUTC
	}
	if req.BreakGlass != nil && !req.BreakGlass.TicketApproved && s.ticketVerifier != nil {
		if approved, err := s.ticketVerifier.VerifyTicketApproval(ctx, req.BreakGlass.TicketID); err == nil && approved {
			bgCopy := *req.BreakGlass
			bgCopy.TicketApproved = true
			req.BreakGlass = &bgCopy
		}
	}
	if s.roleDefRepo != nil && len(req.ResolvedRolePermissions) == 0 {
		if defs, err := s.roleDefRepo.LoadRolePermissions(ctx); err == nil && len(defs) > 0 {
			edges, _ := s.roleDefRepo.GetRoleHierarchyEdges(ctx)
			req.ResolvedRolePermissions = rules.FlattenRoleHierarchyPermissions(req.Subject.Roles, req.Subject.GroupMemberships, defs, edges)
		}
	}
	if len(req.Subject.FlattenedPermissions) == 0 {
		roleDefs := req.ResolvedRolePermissions
		if len(roleDefs) == 0 {
			roleDefs = rules.DefaultRoleDefinitions
		}
		var merged []rbacContracts.Permission
		merged = append(merged, req.Subject.Permissions...)
		for _, r := range req.Subject.Roles {
			merged = append(merged, roleDefs[r]...)
		}
		req.Subject.FlattenedPermissions = rules.BuildPermissionHashset(merged)
	}
	return req
}

func (s *RBACService) IssueSignedTaskLease(subjectID string, taskID string, ttl time.Duration) (string, error) {
	if s.taskLeaseHMACSecret == "" || subjectID == "" || taskID == "" || ttl <= 0 {
		return "", errors.NewValidationError("task lease secret, subjectId, taskId, and positive ttl are required")
	}
	exp := time.Now().UTC().Add(ttl).Unix()
	payload := fmt.Sprintf("%s|%s|%d", subjectID, taskID, exp)
	mac := hmac.New(sha256.New, []byte(s.taskLeaseHMACSecret))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	return payload + "|" + sig, nil
}

func (s *RBACService) VerifySignedTaskLease(leaseToken string, now time.Time) (bool, error) {
	if s.taskLeaseHMACSecret == "" {
		return false, errors.NewUnauthorizedError("task lease secret not configured")
	}
	parts := strings.Split(leaseToken, "|")
	if len(parts) != 4 {
		return false, nil
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || exp <= now.Unix() {
		return false, nil
	}
	payload := parts[0] + "|" + parts[1] + "|" + parts[2]
	mac := hmac.New(sha256.New, []byte(s.taskLeaseHMACSecret))
	mac.Write([]byte(payload))
	expected := mac.Sum(nil)
	provided, err := hex.DecodeString(parts[3])
	if err != nil {
		return false, nil
	}
	return hmac.Equal(expected, provided), nil
}

func (s *RBACService) EvaluateAsyncJobDriftWithOIDC(ctx context.Context, subjectID string, jti string, rawToken string, queueLatency time.Duration) (rbacContracts.AsyncJobRevocationResult, error) {
	var introspectFn func(ctx context.Context) (bool, error)
	if s.oidcIntrospector != nil {
		introspectFn = func(c context.Context) (bool, error) {
			return s.oidcIntrospector.IntrospectToken(c, rawToken)
		}
	}
	return s.EvaluateAsyncJobDrift(ctx, subjectID, jti, queueLatency, introspectFn)
}

func (s *RBACService) ClaimExclusiveRoleBinding(ctx context.Context, tenantID string, subjectID string, role rbacContracts.RoleName) error {
	return observability.WithSpan(ctx, rbacSpanPrefix+".claim_exclusive_role_binding", func(ctx context.Context) error {
		if err := resilience.ValidateDeadlineBudget(ctx, rbacDeadlineMargin, 0); err != nil {
			return err
		}
		if tenantID == "" || subjectID == "" || role == "" {
			return errors.NewValidationError("tenantId, subjectId, and roleName are required")
		}
		if s.exclusiveClaimRepo == nil {
			return errors.NewInternalError("exclusive claim repository is not configured")
		}
		return s.runInUnitOfWork(ctx, func(txCtx context.Context) error {
			if err := s.exclusiveClaimRepo.ClaimExclusiveRoleBinding(txCtx, tenantID, subjectID, role); err != nil {
				return err
			}
			return s.dispatchLifecycleAudit(txCtx, tenantID, subjectID, rbacContracts.ResourceAuth, rbacContracts.ActionWrite, "PERMIT", "exclusive role binding claimed")
		})
	}, attribute.String("tenant_id", tenantID), attribute.String("subject_id", subjectID), attribute.String("role", string(role)))
}

func (s *RBACService) TrackStreamingSession(sessionID string, conn io.Closer, expiresAt time.Time) {
	if sessionID == "" || conn == nil {
		return
	}
	s.streamingSessions.Store(sessionID, activeStreamingSession{
		conn:      conn,
		expiresAt: expiresAt,
		startedAt: time.Now().UTC(),
	})
}

func (s *RBACService) StartStreamingSocketReaper(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now().UTC()
				s.streamingSessions.Range(func(key, value interface{}) bool {
					sessionID := key.(string)
					sess := value.(activeStreamingSession)
					res := rules.EvaluateStreamingSessionHeartbeat(sess.expiresAt, sess.startedAt, now)
					if res.Terminate {
						_ = sess.conn.Close()
						s.streamingSessions.Delete(sessionID)
						observability.GetLogger().Warn(ctx, "terminated expired streaming session",
							observability.String("session_id", sessionID),
							observability.String("duration", res.SessionDuration.String()),
						)
					}
					return true
				})
			}
		}
	}()
}

func (s *RBACService) emitBreakGlassSIEMAlert(ctx context.Context, req rbacContracts.EvaluationRequest, decision rbacContracts.EvaluationResult) {
	if s.siemAlertEmitter == nil {
		return
	}
	_ = s.siemAlertEmitter.EmitSecurityAlert(ctx, rbacContracts.RBACAuditEvent{
		ID:            fmt.Sprintf("rbac-siem-%d", time.Now().UnixNano()),
		TenantID:      req.TargetTenantID,
		SubjectID:     req.Subject.ID,
		Resource:      req.Resource,
		Action:        req.Action,
		Decision:      "ELEVATED_PERMIT",
		Reason:        decision.Reason,
		ViolationCode: decision.ViolationCode,
		OccurredAt:    time.Now().UTC(),
	})
}

func (s *RBACService) loadGrant(ctx context.Context, tenantID string, grantID string) (*rbacContracts.AccessGrantRecord, error) {
	if s.grantRepo == nil {
		return nil, nil
	}
	return s.grantRepo.GetGrant(ctx, tenantID, grantID)
}

func (s *RBACService) findActiveGrants(ctx context.Context, tenantID string, subjectID string) ([]rbacContracts.AccessGrantRecord, error) {
	if tenantID == "" {
		return nil, errors.NewValidationError("tenantId is required")
	}
	if s.grantRepo != nil {
		return s.grantRepo.ListActiveGrants(ctx, tenantID, subjectID)
	}
	return []rbacContracts.AccessGrantRecord{}, nil
}

func (s *RBACService) validateJITPolicy(req rbacContracts.AccessGrantRequest) error {
	return rules.ValidateJITRequest(req)
}

func (s *RBACService) persistGrant(ctx context.Context, record rbacContracts.AccessGrantRecord) error {
	if s.grantRepo == nil {
		return nil
	}
	return s.grantRepo.SaveGrant(ctx, record)
}

func (s *RBACService) persistRevokeGrant(ctx context.Context, tenantID string, grantID string) error {
	if s.grantRepo == nil {
		return nil
	}
	return s.grantRepo.RevokeGrant(ctx, tenantID, grantID)
}

func (s *RBACService) persistUserBinding(ctx context.Context, binding rbacContracts.UserBindingRecord) error {
	if s.bindingRepo == nil {
		return nil
	}
	return s.bindingRepo.BindUserRole(ctx, binding)
}

func (s *RBACService) removeUserBinding(ctx context.Context, tenantID string, subjectID string, role rbacContracts.RoleName) error {
	if s.bindingRepo == nil {
		return nil
	}
	return s.bindingRepo.UnbindUserRole(ctx, tenantID, subjectID, role)
}

func (s *RBACService) dispatchAuditDecision(ctx context.Context, req rbacContracts.EvaluationRequest, decisionLabel string, res rbacContracts.EvaluationResult) error {
	if s.auditOutbox == nil {
		return nil
	}
	return observability.WithSpan(ctx, rbacSpanPrefix+".dispatch_audit_decision", func(ctx context.Context) error {
		return s.auditOutbox.EnqueueAuditEvent(ctx, rbacContracts.RBACAuditEvent{
			ID:            fmt.Sprintf("rbac-aud-%d", time.Now().UnixNano()),
			TenantID:      req.TargetTenantID,
			SubjectID:     req.Subject.ID,
			Resource:      req.Resource,
			Action:        req.Action,
			Decision:      decisionLabel,
			Reason:        res.Reason,
			ViolationCode: res.ViolationCode,
			OccurredAt:    time.Now().UTC(),
		})
	}, attribute.String("rbac.decision", decisionLabel), attribute.String("subject.id", req.Subject.ID), attribute.String("target.tenant_id", req.TargetTenantID))
}

func (s *RBACService) dispatchLifecycleAudit(ctx context.Context, tenantID string, subjectID string, resource rbacContracts.Resource, action rbacContracts.Action, decision string, reason string) error {
	if s.auditOutbox == nil || tenantID == "" || subjectID == "" {
		return nil
	}
	return s.auditOutbox.EnqueueAuditEvent(ctx, rbacContracts.RBACAuditEvent{
		ID:         fmt.Sprintf("rbac-aud-%d", time.Now().UnixNano()),
		TenantID:   tenantID,
		SubjectID:  subjectID,
		Resource:   resource,
		Action:     action,
		Decision:   decision,
		Reason:     reason,
		OccurredAt: time.Now().UTC(),
	})
}

func (s *RBACService) writeDenyFailoverLog(ctx context.Context, req rbacContracts.EvaluationRequest, res rbacContracts.EvaluationResult, outboxErr error) {
	observability.GetLogger().Error(ctx, "rbac deny audit outbox failed; writing local failover log",
		observability.String("subject_id", req.Subject.ID),
		observability.String("error", outboxErr.Error()),
	)
	event := rbacContracts.RBACAuditEvent{
		ID:            fmt.Sprintf("rbac-failover-%d", time.Now().UnixNano()),
		TenantID:      req.TargetTenantID,
		SubjectID:     req.Subject.ID,
		Resource:      req.Resource,
		Action:        req.Action,
		Decision:      "DENY",
		Reason:        res.Reason,
		ViolationCode: res.ViolationCode,
		OccurredAt:    time.Now().UTC(),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	logPath := filepath.Join(os.TempDir(), constants.RBACDenyFailoverLogName)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(payload, '\n'))
}

func transitionGrantForRevoke(currentStatus string) (string, error) {
	next, ok := machines.TransitionGrantStatus(currentStatus, machines.GrantEventRevoke)
	if !ok {
		return currentStatus, errors.NewConflictError("grant cannot transition to REVOKED from current state")
	}
	return next, nil
}

func buildInitialGrantRecord(req rbacContracts.AccessGrantRequest, now time.Time) rbacContracts.AccessGrantRecord {
	expiresAt := now.Add(time.Duration(req.DurationSeconds) * time.Second)
	return rbacContracts.AccessGrantRecord{
		ID:              req.ID,
		TenantID:        req.TenantID,
		SubjectID:       req.SubjectID,
		TargetNamespace: req.TargetNamespace,
		Role:            req.Role,
		Reason:          req.Reason,
		DurationSeconds: req.DurationSeconds,
		ApprovedBy:      req.ApproverRole,
		Status:          "ACTIVE",
		ExpiresAt:       expiresAt,
		IdempotencyKey:  req.IdempotencyKey,
		Version:         1,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func clampPageSize(requested int, max int) int {
	if requested <= 0 {
		return rbacDefaultPageSize
	}
	if requested > max {
		return max
	}
	return requested
}

func decodeCursor(cursor string) int {
	if cursor == "" {
		return 0
	}
	val, err := strconv.Atoi(cursor)
	if err != nil || val < 0 {
		return 0
	}
	return val
}

func shouldBypassPermitCache(req rbacContracts.EvaluationRequest) bool {
	if req.BreakGlass != nil {
		return true
	}
	if strings.EqualFold(req.TargetEnvironment, rbacContracts.EnvProduction) {
		return !strings.EqualFold(string(req.Action), string(rbacContracts.ActionRead))
	}
	return false
}

func buildPermitCacheKey(req rbacContracts.EvaluationRequest) string {
	roles := make([]string, len(req.Subject.Roles))
	for i, r := range req.Subject.Roles {
		roles[i] = string(r)
	}
	return fmt.Sprintf(
		"%s|%s|%s|%s|%s|%s|%s|%t|%t|%t|%s",
		req.Subject.ID,
		req.Subject.TenantID,
		req.TargetTenantID,
		string(req.Resource),
		string(req.Action),
		req.TargetEnvironment,
		req.ResourceClassification,
		req.ChangeFreezeActive,
		req.RequireStepUpMFA,
		req.Subject.MFAVerified,
		strings.Join(roles, ","),
	)
}
