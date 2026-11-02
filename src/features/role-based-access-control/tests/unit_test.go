package tests

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	rbacContracts "internal-developer-platform/src/features/role-based-access-control/contracts"
	"internal-developer-platform/src/features/role-based-access-control/domain/rules"
	rbacAdapters "internal-developer-platform/src/features/role-based-access-control/infrastructure/adapters"
)

// --- UNIT TEST 1: Exhaustive JWT Cryptographic Integrity & Tampering ---
func TestUnit_JWT_CryptographicIntegrity(t *testing.T) {
	secret := "production-grade-hmac-sha256-signing-secret-key-32-bytes"
	verifier := rbacAdapters.NewJWTTokenVerifierAdapter(secret, "idp-unit-test-issuer")

	baseSubject := rbacContracts.Subject{
		ID:        "engineer-alice-101",
		TenantID:  "tenant-core-engineering",
		Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
		ExpiresAt: time.Now().Add(1 * time.Hour),
		JTI:       "jti-token-unit-uuid",
	}

	validToken, err := verifier.IssueToken(baseSubject)
	if err != nil {
		t.Fatalf("failed to issue valid JWT: %v", err)
	}

	tests := []struct {
		name          string
		tokenModifier func(string) string
		expectErr     bool
		errMsgPart    string
	}{
		{
			name: "Valid Token Untampered",
			tokenModifier: func(tok string) string {
				return tok
			},
			expectErr: false,
		},
		{
			name: "Invalid Segments - Single String",
			tokenModifier: func(tok string) string {
				return "invalid-opaque-token"
			},
			expectErr:  true,
			errMsgPart: "invalid token format",
		},
		{
			name: "Invalid Segments - Two Segments",
			tokenModifier: func(tok string) string {
				parts := strings.Split(tok, ".")
				return parts[0] + "." + parts[1]
			},
			expectErr:  true,
			errMsgPart: "invalid token format",
		},
		{
			name: "Tampered Signature",
			tokenModifier: func(tok string) string {
				parts := strings.Split(tok, ".")
				tamperedSig := parts[2] + "tampered"
				return parts[0] + "." + parts[1] + "." + tamperedSig
			},
			expectErr:  true,
			errMsgPart: "cryptographic signature verification failed",
		},
		{
			name: "Tampered Claims Payload (Privilege Escalation Attempt)",
			tokenModifier: func(tok string) string {
				parts := strings.Split(tok, ".")
				// Attempt to tamper payload to PlatformAdmin while keeping original signature
				escalatedJSON := `{"sub":"engineer-alice-101","tenantId":"tenant-core-engineering","roles":["PlatformAdmin"],"iss":"idp-unit-test-issuer","exp":9999999999}`
				escalatedB64 := base64.RawURLEncoding.EncodeToString([]byte(escalatedJSON))
				return parts[0] + "." + escalatedB64 + "." + parts[2]
			},
			expectErr:  true,
			errMsgPart: "cryptographic signature verification failed",
		},
		{
			name: "Expired Token",
			tokenModifier: func(tok string) string {
				expiredSub := baseSubject
				expiredSub.ExpiresAt = time.Now().Add(-5 * time.Minute)
				expTok, _ := verifier.IssueToken(expiredSub)
				return expTok
			},
			expectErr:  true,
			errMsgPart: "token has expired",
		},
		{
			name: "Different Signing Secret (Signature Mismatch)",
			tokenModifier: func(tok string) string {
				rogueVerifier := rbacAdapters.NewJWTTokenVerifierAdapter("attacker-secret-key", "idp-unit-test-issuer")
				rogueTok, _ := rogueVerifier.IssueToken(baseSubject)
				return rogueTok
			},
			expectErr:  true,
			errMsgPart: "cryptographic signature verification failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tokenToTest := tc.tokenModifier(validToken)
			sub, err := verifier.VerifyToken(context.Background(), tokenToTest)

			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error containing %q, but verification succeeded", tc.errMsgPart)
				}
				if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.errMsgPart)) {
					t.Fatalf("expected error containing %q, got %q", tc.errMsgPart, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("expected valid token, but got error: %v", err)
				}
				if sub.ID != baseSubject.ID {
					t.Errorf("expected subject id %q, got %q", baseSubject.ID, sub.ID)
				}
				if sub.TenantID != baseSubject.TenantID {
					t.Errorf("expected tenant id %q, got %q", baseSubject.TenantID, sub.TenantID)
				}
			}
		})
	}
}

// --- UNIT TEST 2: Comprehensive Authorization Decision Matrix ---
func TestUnit_DomainAuthorization_Matrix(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name           string
		subject        rbacContracts.Subject
		targetTenant   string
		resource       rbacContracts.Resource
		action         rbacContracts.Action
		expectedGrant  bool
		expectedReason string
	}{
		{
			name: "PlatformAdmin can perform ANY action on ANY resource in ANY tenant",
			subject: rbacContracts.Subject{
				ID:        "root-admin",
				TenantID:  "system-tenant",
				Roles:     []rbacContracts.RoleName{rbacContracts.RolePlatformAdmin},
				ExpiresAt: now.Add(1 * time.Hour),
			},
			targetTenant:   "arbitrary-customer-tenant",
			resource:       rbacContracts.ResourceServiceCatalog,
			action:         rbacContracts.ActionDelete,
			expectedGrant:  true,
			expectedReason: "permission granted",
		},
		{
			name: "Developer can read service-catalog in own tenant",
			subject: rbacContracts.Subject{
				ID:        "dev-bob",
				TenantID:  "tenant-alpha",
				Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
				ExpiresAt: now.Add(1 * time.Hour),
			},
			targetTenant:   "tenant-alpha",
			resource:       rbacContracts.ResourceServiceCatalog,
			action:         rbacContracts.ActionRead,
			expectedGrant:  true,
			expectedReason: "permission granted",
		},
		{
			name: "Developer can write service-catalog in own tenant",
			subject: rbacContracts.Subject{
				ID:        "dev-bob",
				TenantID:  "tenant-alpha",
				Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
				ExpiresAt: now.Add(1 * time.Hour),
			},
			targetTenant:   "tenant-alpha",
			resource:       rbacContracts.ResourceServiceCatalog,
			action:         rbacContracts.ActionWrite,
			expectedGrant:  true,
			expectedReason: "permission granted",
		},
		{
			name: "Developer can update service-catalog in own tenant",
			subject: rbacContracts.Subject{
				ID:        "dev-bob",
				TenantID:  "tenant-alpha",
				Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
				ExpiresAt: now.Add(1 * time.Hour),
			},
			targetTenant:   "tenant-alpha",
			resource:       rbacContracts.ResourceServiceCatalog,
			action:         rbacContracts.ActionUpdate,
			expectedGrant:  true,
			expectedReason: "permission granted",
		},
		{
			name: "Developer DENIED delete action on service-catalog (Delete requires admin)",
			subject: rbacContracts.Subject{
				ID:        "dev-bob",
				TenantID:  "tenant-alpha",
				Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
				ExpiresAt: now.Add(1 * time.Hour),
			},
			targetTenant:   "tenant-alpha",
			resource:       rbacContracts.ResourceServiceCatalog,
			action:         rbacContracts.ActionDelete,
			expectedGrant:  false,
			expectedReason: "insufficient permissions",
		},
		{
			name: "Developer DENIED cross-tenant access even for read action",
			subject: rbacContracts.Subject{
				ID:        "dev-bob",
				TenantID:  "tenant-alpha",
				Roles:     []rbacContracts.RoleName{rbacContracts.RoleDeveloper},
				ExpiresAt: now.Add(1 * time.Hour),
			},
			targetTenant:   "tenant-beta",
			resource:       rbacContracts.ResourceServiceCatalog,
			action:         rbacContracts.ActionRead,
			expectedGrant:  false,
			expectedReason: "tenant isolation violation",
		},
		{
			name: "Viewer GRANTED read on service-catalog in own tenant",
			subject: rbacContracts.Subject{
				ID:        "auditor-carol",
				TenantID:  "tenant-alpha",
				Roles:     []rbacContracts.RoleName{rbacContracts.RoleViewer},
				ExpiresAt: now.Add(1 * time.Hour),
			},
			targetTenant:   "tenant-alpha",
			resource:       rbacContracts.ResourceServiceCatalog,
			action:         rbacContracts.ActionRead,
			expectedGrant:  true,
			expectedReason: "permission granted",
		},
		{
			name: "Viewer DENIED write on service-catalog in own tenant",
			subject: rbacContracts.Subject{
				ID:        "auditor-carol",
				TenantID:  "tenant-alpha",
				Roles:     []rbacContracts.RoleName{rbacContracts.RoleViewer},
				ExpiresAt: now.Add(1 * time.Hour),
			},
			targetTenant:   "tenant-alpha",
			resource:       rbacContracts.ResourceServiceCatalog,
			action:         rbacContracts.ActionWrite,
			expectedGrant:  false,
			expectedReason: "insufficient permissions",
		},
		{
			name: "Custom Fine-Grained Permissions attached directly to Subject take effect",
			subject: rbacContracts.Subject{
				ID:       "special-agent",
				TenantID: "tenant-delta",
				Roles:    []rbacContracts.RoleName{}, // No roles
				Permissions: []rbacContracts.Permission{
					{Resource: rbacContracts.ResourceSecrets, Action: rbacContracts.ActionRead},
				},
				ExpiresAt: now.Add(1 * time.Hour),
			},
			targetTenant:   "tenant-delta",
			resource:       rbacContracts.ResourceSecrets,
			action:         rbacContracts.ActionRead,
			expectedGrant:  true,
			expectedReason: "permission granted",
		},
		{
			name: "Custom Fine-Grained Permissions denied for unassigned resource",
			subject: rbacContracts.Subject{
				ID:       "special-agent",
				TenantID: "tenant-delta",
				Roles:    []rbacContracts.RoleName{},
				Permissions: []rbacContracts.Permission{
					{Resource: rbacContracts.ResourceSecrets, Action: rbacContracts.ActionRead},
				},
				ExpiresAt: now.Add(1 * time.Hour),
			},
			targetTenant:   "tenant-delta",
			resource:       rbacContracts.ResourceServiceCatalog,
			action:         rbacContracts.ActionRead,
			expectedGrant:  false,
			expectedReason: "insufficient permissions",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := rbacContracts.EvaluationRequest{
				Subject:        tc.subject,
				TargetTenantID: tc.targetTenant,
				Resource:       tc.resource,
				Action:         tc.action,
			}
			result := rules.EvaluateAuthorization(req, now)

			if result.Granted != tc.expectedGrant {
				t.Fatalf("expected Granted=%v, got Granted=%v (Reason: %s)", tc.expectedGrant, result.Granted, result.Reason)
			}
			if !strings.Contains(strings.ToLower(result.Reason), strings.ToLower(tc.expectedReason)) {
				t.Fatalf("expected reason containing %q, got %q", tc.expectedReason, result.Reason)
			}
		})
	}
}

// --- UNIT TEST 3: In-Memory Revocation Store Edge Cases ---
func TestUnit_MemoryRevocationAdapter(t *testing.T) {
	ctx := context.Background()
	store := rbacAdapters.NewMemoryRevocationAdapter()

	// Initial check
	revoked, err := store.IsRevoked(ctx, "sub-1", "jti-1")
	if err != nil || revoked {
		t.Fatalf("expected false, got revoked=%v, err=%v", revoked, err)
	}

	// Revoke Sub
	_ = store.RevokeSubject(ctx, "sub-1")
	revoked, _ = store.IsRevoked(ctx, "sub-1", "")
	if !revoked {
		t.Errorf("expected sub-1 to be revoked")
	}

	// Revoke JTI
	_ = store.RevokeJTI(ctx, "jti-xyz")
	revoked, _ = store.IsRevoked(ctx, "different-sub", "jti-xyz")
	if !revoked {
		t.Errorf("expected jti-xyz to be revoked")
	}
}
