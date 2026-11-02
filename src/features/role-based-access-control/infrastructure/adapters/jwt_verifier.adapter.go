package adapters

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/shared/errors"
)

type JWTClaims struct {
	Sub         string                   `json:"sub"`
	TenantID    string                   `json:"tenantId"`
	Roles       []rbacContracts.RoleName `json:"roles"`
	Permissions []rbacContracts.Permission `json:"permissions,omitempty"`
	Iss         string                   `json:"iss"`
	Exp         int64                    `json:"exp"`
	Jti         string                   `json:"jti,omitempty"`
}

type JWTTokenVerifierAdapter struct {
	secretKey []byte
	issuer    string
}

func NewJWTTokenVerifierAdapter(secret string, issuer string) *JWTTokenVerifierAdapter {
	if secret == "" {
		secret = "internal-developer-platform-dev-signing-secret"
	}
	if issuer == "" {
		issuer = "internal-developer-platform-idp"
	}
	return &JWTTokenVerifierAdapter{
		secretKey: []byte(secret),
		issuer:    issuer,
	}
}

func (a *JWTTokenVerifierAdapter) IssueToken(subject rbacContracts.Subject) (string, error) {
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}
	headerBytes, _ := json.Marshal(header)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerBytes)

	exp := subject.ExpiresAt.Unix()
	if exp == 0 {
		exp = time.Now().Add(24 * time.Hour).Unix()
	}

	claims := JWTClaims{
		Sub:         subject.ID,
		TenantID:    subject.TenantID,
		Roles:       subject.Roles,
		Permissions: subject.Permissions,
		Iss:         a.issuer,
		Exp:         exp,
		Jti:         subject.JTI,
	}
	claimsBytes, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("failed to marshal claims: %w", err)
	}
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsBytes)

	signingInput := headerB64 + "." + claimsB64
	sig := a.sign(signingInput)
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	return signingInput + "." + sigB64, nil
}

func (a *JWTTokenVerifierAdapter) VerifyToken(ctx context.Context, rawToken string) (*rbacContracts.Subject, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return nil, errors.NewUnauthorizedError("invalid token format: must have 3 segments")
	}

	signingInput := parts[0] + "." + parts[1]
	expectedSig := a.sign(signingInput)
	providedSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(expectedSig, providedSig) {
		return nil, errors.NewUnauthorizedError("cryptographic signature verification failed")
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.NewUnauthorizedError("malformed claims payload in token")
	}

	var claims JWTClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, errors.NewUnauthorizedError("invalid json claims in token")
	}

	expTime := time.Unix(claims.Exp, 0)
	if time.Now().After(expTime) {
		return nil, errors.NewUnauthorizedError("token has expired")
	}

	return &rbacContracts.Subject{
		ID:          claims.Sub,
		TenantID:    claims.TenantID,
		Roles:       claims.Roles,
		Permissions: claims.Permissions,
		Issuer:      claims.Iss,
		ExpiresAt:   expTime,
		JTI:         claims.Jti,
	}, nil
}

func (a *JWTTokenVerifierAdapter) sign(input string) []byte {
	h := hmac.New(sha256.New, a.secretKey)
	h.Write([]byte(input))
	return h.Sum(nil)
}
