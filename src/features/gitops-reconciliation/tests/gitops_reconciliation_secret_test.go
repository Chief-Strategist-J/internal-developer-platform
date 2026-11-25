package tests

import (
	"context"
	"testing"

	"internal-developer-platform/src/features/gitops-reconciliation/contracts"
	"internal-developer-platform/src/features/gitops-reconciliation/domain/rules"
	"internal-developer-platform/src/features/gitops-reconciliation/infrastructure/adapters"
)

func TestValidateSecretMechanismConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     contracts.SecretBackendConfig
		wantErr bool
	}{
		{
			name: "valid sealed secrets",
			cfg: contracts.SecretBackendConfig{
				TenantID:         "tenant-1",
				Mechanism:        contracts.SecretMechanismSealedSecrets,
				EncryptionKeyRef: "sealed-secrets-key",
				Enabled:          true,
			},
			wantErr: false,
		},
		{
			name: "valid external secrets operator",
			cfg: contracts.SecretBackendConfig{
				TenantID:       "tenant-1",
				Mechanism:      contracts.SecretMechanismExternalSecretsOperator,
				SecretStoreRef: "vault-backend",
				Enabled:        true,
			},
			wantErr: false,
		},
		{
			name: "valid secrets store csi driver",
			cfg: contracts.SecretBackendConfig{
				TenantID:      "tenant-1",
				Mechanism:     contracts.SecretMechanismSecretsStoreCSI,
				CSIDriverName: "secrets-store.csi.k8s.io",
				Enabled:       true,
			},
			wantErr: false,
		},
		{
			name: "missing tenantId",
			cfg: contracts.SecretBackendConfig{
				Mechanism:        contracts.SecretMechanismSealedSecrets,
				EncryptionKeyRef: "sealed-secrets-key",
			},
			wantErr: true,
		},
		{
			name: "unsupported mechanism",
			cfg: contracts.SecretBackendConfig{
				TenantID:  "tenant-1",
				Mechanism: "UNKNOWN",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := rules.ValidateSecretMechanismConfig(tc.cfg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateSecretMechanismConfig() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestEvaluateSecretInGitOpsPath(t *testing.T) {
	blockedPlaintext := rules.EvaluateSecretInGitOpsPath(contracts.SecretMechanismExternalSecretsOperator, true, false)
	if blockedPlaintext.Allowed || blockedPlaintext.ViolationCode != "P0_03_PLAINTEXT_SECRET_IN_GIT" {
		t.Fatalf("expected plaintext secret to be blocked, got %+v", blockedPlaintext)
	}

	blockedPlugin := rules.EvaluateSecretInGitOpsPath(contracts.SecretMechanismExternalSecretsOperator, false, true)
	if blockedPlugin.Allowed || blockedPlugin.ViolationCode != "P0_03_PLUGIN_SECRET_INJECTION" {
		t.Fatalf("expected plugin injection to be blocked, got %+v", blockedPlugin)
	}

	allowed := rules.EvaluateSecretInGitOpsPath(contracts.SecretMechanismExternalSecretsOperator, false, false)
	if !allowed.Allowed {
		t.Fatalf("expected valid secret reference to be allowed, got %+v", allowed)
	}
}

func TestConfigurableSecretAdapter(t *testing.T) {
	ctx := context.Background()
	adapter := adapters.NewConfigurableSecretAdapter()

	cfg := contracts.SecretBackendConfig{
		TenantID:       "tenant-eso",
		Mechanism:      contracts.SecretMechanismExternalSecretsOperator,
		SecretStoreRef: "aws-secrets-manager",
		Enabled:        true,
	}

	saved, err := adapter.ConfigureSecretMechanism(ctx, cfg)
	if err != nil {
		t.Fatalf("ConfigureSecretMechanism() unexpected error: %v", err)
	}
	if saved.Mechanism != contracts.SecretMechanismExternalSecretsOperator {
		t.Fatalf("expected mechanism EXTERNAL_SECRETS_OPERATOR, got %s", saved.Mechanism)
	}

	resolved, err := adapter.ResolveSecretReference(ctx, "tenant-eso", contracts.SecretReference{
		Name:         "db-creds",
		Namespace:    "prod",
		Mechanism:    contracts.SecretMechanismExternalSecretsOperator,
		RemoteKeyRef: "prod/db/password",
	})
	if err != nil {
		t.Fatalf("ResolveSecretReference() unexpected error: %v", err)
	}
	if resolved["kind"] != "ExternalSecret" || resolved["secretStoreRef"] != "aws-secrets-manager" {
		t.Fatalf("unexpected resolved reference: %+v", resolved)
	}
}
