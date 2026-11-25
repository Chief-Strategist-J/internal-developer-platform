/*
ALGORITHM & ARCHITECTURE BLUEPRINT: RBAC JWT ADAPTER (NATIVE GOLANG-JWT)

1. Architecture Role:
  - Step 11 Adapter: JWT cryptographic token issuer and verifier implementing ports.TokenVerifierPort.
  - Uses github.com/golang-jwt/jwt/v5 with HS256 signing and strict expiration/method validation.
  - Preserves Subject, TenantID, Roles, GroupMemberships, ClientType, MFAVerified, Classification, and JTI.
  - Zero mid-function inline comments.
*/
package adapters

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/infra/observability"
	"internal-developer-platform/src/shared/constants"
	"internal-developer-platform/src/shared/errors"
)

type JWTClaims struct {
	TenantID         string                     `json:"tenantId"`
	Roles            []rbacContracts.RoleName   `json:"roles"`
	GroupMemberships []string                   `json:"groupMemberships,omitempty"`
	ClientType       string                     `json:"clientType,omitempty"`
	Permissions      []rbacContracts.Permission `json:"permissions,omitempty"`
	MFAVerified      bool                       `json:"mfaVerified,omitempty"`
	Classification   string                     `json:"classification,omitempty"`
	jwt.RegisteredClaims
}

type JWTTokenVerifierAdapter struct {
	secretKey []byte
	issuer    string
}

func NewJWTTokenVerifierAdapter(secret string, issuer string) *JWTTokenVerifierAdapter {
	if issuer == "" {
		issuer = constants.DefaultJWTIssuer
	}
	return &JWTTokenVerifierAdapter{
		secretKey: []byte(secret),
		issuer:    issuer,
	}
}

func (a *JWTTokenVerifierAdapter) IssueToken(subject rbacContracts.Subject) (string, error) {
	ctx := context.Background()
	return observability.RunTraced(ctx, "jwt.token.sign", func(ctx context.Context) (string, error) {
		if len(a.secretKey) == 0 {
			return "", errors.NewUnauthorizedError("jwt signing secret is not configured")
		}
		expTime := subject.ExpiresAt
		if expTime.IsZero() {
			expTime = time.Now().Add(2 * time.Hour)
		}
		claims := JWTClaims{
			TenantID:         subject.TenantID,
			Roles:            subject.Roles,
			GroupMemberships: subject.GroupMemberships,
			ClientType:       subject.ClientType,
			Permissions:      subject.Permissions,
			MFAVerified:      subject.MFAVerified,
			Classification:   subject.Classification,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   subject.ID,
				Issuer:    a.issuer,
				ExpiresAt: jwt.NewNumericDate(expTime),
				IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
				ID:        subject.JTI,
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString(a.secretKey)
		if err != nil {
			return "", fmt.Errorf("failed to sign jwt token: %w", err)
		}
		return signed, nil
	})
}

func (a *JWTTokenVerifierAdapter) VerifyToken(ctx context.Context, rawToken string) (*rbacContracts.Subject, error) {
	return observability.RunTraced(ctx, "jwt.token.verify", func(ctx context.Context) (*rbacContracts.Subject, error) {
		if len(a.secretKey) == 0 {
			return nil, errors.NewUnauthorizedError("jwt signing secret is not configured")
		}
		if rawToken == "" {
			return nil, errors.NewUnauthorizedError("missing bearer token")
		}
		claims := &JWTClaims{}
		parsed, err := jwt.ParseWithClaims(
			rawToken,
			claims,
			func(token *jwt.Token) (interface{}, error) {
				return a.secretKey, nil
			},
			jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			jwt.WithExpirationRequired(),
		)
		if err != nil || !parsed.Valid {
			return nil, errors.NewUnauthorizedError("invalid or expired jwt token")
		}
		var expTime time.Time
		if claims.ExpiresAt != nil {
			expTime = claims.ExpiresAt.Time
		}
		return &rbacContracts.Subject{
			ID:               claims.Subject,
			TenantID:         claims.TenantID,
			Roles:            claims.Roles,
			GroupMemberships: claims.GroupMemberships,
			ClientType:       claims.ClientType,
			Permissions:      claims.Permissions,
			MFAVerified:      claims.MFAVerified,
			Classification:   claims.Classification,
			Issuer:           claims.Issuer,
			ExpiresAt:        expTime,
			JTI:              claims.ID,
		}, nil
	})
}
