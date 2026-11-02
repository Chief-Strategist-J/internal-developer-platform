package tests

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	rbacAdapters "internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
)

func TestRoleBasedAccessControlSchema_CryptographicSignatureAndPayload(t *testing.T) {
	secret := "strict-canonical-schema-test-secret-key-32b"
	verifier := rbacAdapters.NewJWTTokenVerifierAdapter(secret, "idp-schema-test")

	sub := rbacContracts.Subject{
		ID:        "eng-1",
		TenantID:  "tenant-core",
		Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
		ExpiresAt: time.Now().Add(1 * time.Hour),
		JTI:       "schema-jti-uuid",
	}

	token, err := verifier.IssueToken(sub)
	if err != nil {
		t.Fatalf("failed to issue token: %v", err)
	}

	parsedSub, err := verifier.VerifyToken(context.Background(), token)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}
	if parsedSub.ID != sub.ID {
		t.Errorf("expected subject %s, got %s", sub.ID, parsedSub.ID)
	}

	parts := strings.Split(token, ".")
	tamperedPayload := parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"eng-1","roles":["PlatformAdmin"],"exp":9999999999}`)) + "." + parts[2]
	_, err = verifier.VerifyToken(context.Background(), tamperedPayload)
	if err == nil {
		t.Fatalf("expected signature verification failure for tampered payload")
	}
}
