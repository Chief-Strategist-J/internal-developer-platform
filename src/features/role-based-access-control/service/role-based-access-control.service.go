package service

import (
	"context"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/features/role-based-access-control/domain/rules"
	"internal-developer-platform/src/features/role-based-access-control/infrastructure/ports"
	"internal-developer-platform/src/shared/errors"
)

type RBACService struct {
	tokenVerifier   ports.TokenVerifierPort
	revocationStore ports.RevocationStorePort
	grantRepo       ports.AccessGrantRepositoryPort
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

func (s *RBACService) AuthenticateToken(ctx context.Context, rawToken string) (*rbacContracts.Subject, error) {
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
}

func (s *RBACService) Authorize(ctx context.Context, subject rbacContracts.Subject, targetTenant string, resource rbacContracts.Resource, action rbacContracts.Action) error {
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
}

func (s *RBACService) AuthenticateAndAuthorize(ctx context.Context, rawToken string, targetTenant string, resource rbacContracts.Resource, action rbacContracts.Action) (*rbacContracts.Subject, error) {
	subject, err := s.AuthenticateToken(ctx, rawToken)
	if err != nil {
		return nil, err
	}
	if err := s.Authorize(ctx, *subject, targetTenant, resource, action); err != nil {
		return nil, err
	}
	return subject, nil
}

func (s *RBACService) IssueToken(subject rbacContracts.Subject) (string, error) {
	return s.tokenVerifier.IssueToken(subject)
}

func (s *RBACService) RevokeSubject(ctx context.Context, subjectID string) error {
	if s.revocationStore != nil {
		return s.revocationStore.RevokeSubject(ctx, subjectID)
	}
	return nil
}

func (s *RBACService) RequestJITAccess(ctx context.Context, req rbacContracts.AccessGrantRequest) (*rbacContracts.AccessGrantRecord, error) {
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
}

func (s *RBACService) RevokeJITAccess(ctx context.Context, tenantID, grantID string) error {
	if tenantID == "" || grantID == "" {
		return errors.NewValidationError("tenantId and grantId are required")
	}
	if s.grantRepo != nil {
		return s.grantRepo.RevokeGrant(ctx, tenantID, grantID)
	}
	return nil
}

func (s *RBACService) ListActiveGrants(ctx context.Context, tenantID, subjectID string) ([]rbacContracts.AccessGrantRecord, error) {
	if tenantID == "" {
		return nil, errors.NewValidationError("tenantId is required")
	}
	if s.grantRepo != nil {
		return s.grantRepo.ListActiveGrants(ctx, tenantID, subjectID)
	}
	return []rbacContracts.AccessGrantRecord{}, nil
}
