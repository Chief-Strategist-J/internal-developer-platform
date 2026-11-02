package ports

import (
	"context"
	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
)

type RevocationStorePort interface {
	IsRevoked(ctx context.Context, subjectID string, jti string) (bool, error)
	RevokeSubject(ctx context.Context, subjectID string) error
	RevokeJTI(ctx context.Context, jti string) error
}

type TokenVerifierPort interface {
	VerifyToken(ctx context.Context, rawToken string) (*rbacContracts.Subject, error)
	IssueToken(subject rbacContracts.Subject) (string, error)
}
