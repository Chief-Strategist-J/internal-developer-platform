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
}

func NewRBACService(verifier ports.TokenVerifierPort, revocation ports.RevocationStorePort) *RBACService {
	return &RBACService{
		tokenVerifier:   verifier,
		revocationStore: revocation,
	}
}

// AuthenticateToken verifies the signature, expiration, and checks the revocation store
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

// Authorize evaluates domain rules against the subject, resource, and action
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

// AuthenticateAndAuthorize executes the complete pipeline: verifies token, checks revocation, and enforces permissions
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
