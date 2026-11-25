// Package service provides domain authorization and token management for RBAC.
//
// Architectural Blueprint:
// - Step 06 Service: Evaluates subject permissions, verifies tokens, and governs JIT grants.
// - Traced with OpenTelemetry child spans using observability.RunTraced and observability.WithSpan.
// - 100% comment-free function bodies.
package service

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/features/role-based-access-control/domain/rules"
	"internal-developer-platform/src/features/role-based-access-control/infrastructure/ports"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/errors"
)

type RBACService struct {
	tokenVerifier   ports.TokenVerifierPort
	revocationStore ports.RevocationStorePort
	grantRepo       ports.AccessGrantRepositoryPort
	bindingRepo     ports.UserBindingRepositoryPort
	auditOutbox     ports.RBACAuditOutboxPort
}

func NewRBACService(verifier ports.TokenVerifierPort, revocation ports.RevocationStorePort, grantRepo ...ports.AccessGrantRepositoryPort) *RBACService {
	svc := &RBACService{
		tokenVerifier:   verifier,
		revocationStore: revocation,
	}
	if len(grantRepo) > 0 && grantRepo[0] != nil {
		svc.grantRepo = grantRepo[0]
	}
	return svc
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

func (s *RBACService) AuthenticateToken(ctx context.Context, rawToken string) (*rbacContracts.Subject, error) {
	return observability.RunTraced(ctx, "rbac.service.authenticate_token", func(ctx context.Context) (*rbacContracts.Subject, error) {
		subject, err := s.tokenVerifier.VerifyToken(ctx, rawToken)
		if err != nil {
			return nil, err
		}
		if s.revocationStore != nil {
			revoked, err := s.revocationStore.IsRevoked(ctx, subject.ID, subject.JTI)
			if err == nil && revoked {
				return nil, errors.NewUnauthorizedError("subject or token has been revoked")
			}
		}
		return subject, nil
	})
}

func (s *RBACService) Authorize(ctx context.Context, subject rbacContracts.Subject, targetTenant string, resource rbacContracts.Resource, action rbacContracts.Action) error {
	return observability.WithSpan(ctx, "rbac.service.authorize", func(ctx context.Context) error {
		req := rbacContracts.EvaluationRequest{
			Subject:        subject,
			TargetTenantID: targetTenant,
			Resource:       resource,
			Action:         action,
		}
		decision := rules.EvaluateAuthorization(req, time.Now())
		if !decision.Granted {
			return errors.NewForbiddenError(decision.Reason)
		}
		return nil
	}, attribute.String("subject.id", subject.ID), attribute.String("target_tenant", targetTenant), attribute.String("resource", string(resource)), attribute.String("action", string(action)))
}

func (s *RBACService) AuthenticateAndAuthorize(ctx context.Context, rawToken string, targetTenant string, resource rbacContracts.Resource, action rbacContracts.Action) (*rbacContracts.Subject, error) {
	return observability.RunTraced(ctx, "rbac.service.authenticate_and_authorize", func(ctx context.Context) (*rbacContracts.Subject, error) {
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
	return observability.RunTraced(ctx, "rbac.service.issue_token", func(ctx context.Context) (string, error) {
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
	return observability.WithSpan(ctx, "rbac.service.revoke_subject", func(ctx context.Context) error {
		if s.revocationStore != nil {
			return s.revocationStore.RevokeSubject(ctx, subjectID)
		}
		return nil
	}, attribute.String("subject.id", subjectID))
}

func (s *RBACService) RequestJITAccess(ctx context.Context, req rbacContracts.AccessGrantRequest) (*rbacContracts.AccessGrantRecord, error) {
	return observability.RunTraced(ctx, "rbac.service.request_jit_access", func(ctx context.Context) (*rbacContracts.AccessGrantRecord, error) {
		if err := rules.ValidateJITRequest(req); err != nil {
			return nil, err
		}
		now := time.Now()
		expiresAt := now.Add(time.Duration(req.DurationSeconds) * time.Second)
		record := rbacContracts.AccessGrantRecord{
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
		if s.grantRepo != nil {
			if err := s.grantRepo.SaveGrant(ctx, record); err != nil {
				return nil, err
			}
		}
		return &record, nil
	}, attribute.String("tenant_id", req.TenantID), attribute.String("subject.id", req.SubjectID), attribute.String("role", req.Role))
}

func (s *RBACService) RevokeJITAccess(ctx context.Context, tenantID, grantID string) error {
	return observability.WithSpan(ctx, "rbac.service.revoke_jit_access", func(ctx context.Context) error {
		if tenantID == "" || grantID == "" {
			return errors.NewValidationError("tenantId and grantId are required")
		}
		if s.grantRepo != nil {
			return s.grantRepo.RevokeGrant(ctx, tenantID, grantID)
		}
		return nil
	}, attribute.String("tenant_id", tenantID), attribute.String("grant_id", grantID))
}

func (s *RBACService) ListActiveGrants(ctx context.Context, tenantID, subjectID string) ([]rbacContracts.AccessGrantRecord, error) {
	return observability.RunTraced(ctx, "rbac.service.list_active_grants", func(ctx context.Context) ([]rbacContracts.AccessGrantRecord, error) {
		if tenantID == "" {
			return nil, errors.NewValidationError("tenantId is required")
		}
		if s.grantRepo != nil {
			return s.grantRepo.ListActiveGrants(ctx, tenantID, subjectID)
		}
		return []rbacContracts.AccessGrantRecord{}, nil
	}, attribute.String("tenant_id", tenantID), attribute.String("subject.id", subjectID))
}

func (s *RBACService) BindUserRole(ctx context.Context, binding rbacContracts.UserBindingRecord) error {
	return observability.WithSpan(ctx, "rbac.service.bind_user_role", func(ctx context.Context) error {
		if binding.TenantID == "" || binding.SubjectID == "" || binding.RoleName == "" {
			return errors.NewValidationError("tenantId, subjectId, and roleName are required")
		}
		if s.bindingRepo != nil {
			return s.bindingRepo.BindUserRole(ctx, binding)
		}
		return nil
	}, attribute.String("tenant_id", binding.TenantID), attribute.String("subject_id", binding.SubjectID), attribute.String("role", string(binding.RoleName)))
}

func (s *RBACService) UnbindUserRole(ctx context.Context, tenantID string, subjectID string, role rbacContracts.RoleName) error {
	return observability.WithSpan(ctx, "rbac.service.unbind_user_role", func(ctx context.Context) error {
		if tenantID == "" || subjectID == "" || role == "" {
			return errors.NewValidationError("tenantId, subjectId, and roleName are required")
		}
		if s.bindingRepo != nil {
			return s.bindingRepo.UnbindUserRole(ctx, tenantID, subjectID, role)
		}
		return nil
	}, attribute.String("tenant_id", tenantID), attribute.String("subject_id", subjectID), attribute.String("role", string(role)))
}

func (s *RBACService) GetUserRoles(ctx context.Context, tenantID string, subjectID string) ([]rbacContracts.RoleName, error) {
	return observability.RunTraced(ctx, "rbac.service.get_user_roles", func(ctx context.Context) ([]rbacContracts.RoleName, error) {
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
	return observability.RunTraced(ctx, "rbac.service.authorize_with_context", func(ctx context.Context) (rbacContracts.EvaluationResult, error) {
		decision := rules.EvaluateAuthorization(req, time.Now())
		if !decision.Granted {
			s.dispatchAuditDecision(ctx, req, "DENY", decision)
			return decision, errors.NewForbiddenError(decision.Reason)
		}
		if decision.ElevatedPermit {
			if err := s.dispatchAuditDecision(ctx, req, "ELEVATED_PERMIT", decision); err != nil {
				return rbacContracts.EvaluationResult{
					Granted:       false,
					Reason:        "cannot grant break-glass access without audit guarantee",
					ViolationCode: "ERR_AUDIT_UNAVAILABLE",
				}, errors.NewForbiddenError("cannot grant break-glass access without audit guarantee")
			}
		}
		return decision, nil
	}, attribute.String("subject.id", req.Subject.ID), attribute.String("target_tenant", req.TargetTenantID), attribute.String("resource", string(req.Resource)))
}

func (s *RBACService) ValidateRoleHierarchy(ctx context.Context, edges map[string][]string) error {
	return observability.WithSpan(ctx, "rbac.service.validate_role_hierarchy", func(ctx context.Context) error {
		return rules.ValidateRoleHierarchyDAG(edges)
	})
}

func (s *RBACService) VerifyImpersonation(ctx context.Context, callerMTLS bool, hasImpersonateGrant bool, targetProtectedAdmin bool) error {
	return observability.WithSpan(ctx, "rbac.service.verify_impersonation", func(ctx context.Context) error {
		return rules.EvaluateImpersonation(callerMTLS, hasImpersonateGrant, targetProtectedAdmin)
	})
}

func (s *RBACService) VerifyDelegationScope(ctx context.Context, isTenantDelegation bool, tokenTenantID string, targetTenantID string) error {
	return observability.WithSpan(ctx, "rbac.service.verify_delegation_scope", func(ctx context.Context) error {
		return rules.EvaluateDelegationScope(isTenantDelegation, tokenTenantID, targetTenantID)
	})
}

func (s *RBACService) dispatchAuditDecision(ctx context.Context, req rbacContracts.EvaluationRequest, decisionLabel string, res rbacContracts.EvaluationResult) error {
	if s.auditOutbox == nil {
		return nil
	}
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
}
