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

type AccessGrantRepositoryPort interface {
	SaveGrant(ctx context.Context, grant rbacContracts.AccessGrantRecord) error
	GetGrant(ctx context.Context, tenantID string, id string) (*rbacContracts.AccessGrantRecord, error)
	RevokeGrant(ctx context.Context, tenantID string, id string) error
	ListActiveGrants(ctx context.Context, tenantID string, subjectID string) ([]rbacContracts.AccessGrantRecord, error)
}

type UserBindingRepositoryPort interface {
	BindUserRole(ctx context.Context, binding rbacContracts.UserBindingRecord) error
	UnbindUserRole(ctx context.Context, tenantID string, subjectID string, role rbacContracts.RoleName) error
	GetUserRoles(ctx context.Context, tenantID string, subjectID string) ([]rbacContracts.RoleName, error)
}

type RBACAuditOutboxPort interface {
	EnqueueAuditEvent(ctx context.Context, event rbacContracts.RBACAuditEvent) error
}

type UnitOfWorkPort interface {
	Run(ctx context.Context, fn func(txCtx context.Context) error) error
}
