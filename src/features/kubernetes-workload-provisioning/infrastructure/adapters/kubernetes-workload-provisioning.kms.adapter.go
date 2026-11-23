// Package adapters provides outbound infrastructure adapters for Kubernetes Workload Provisioning.
//
// Architectural Blueprint:
// - Step 11 Adapter: Implements KmsEncryptionPort using the infrastructure KMS EnvelopeEncryptionEngine.
// - Wraps concrete envelope encryption calls and returns domain-friendly ciphertext strings.
// - Adheres to Inversion of Control: Domain service depends strictly on ports.KmsEncryptionPort.
package adapters

import (
	"context"

	"internal-developer-platform/src/features/kubernetes-workload-provisioning/infrastructure/ports"
	"internal-developer-platform/src/infra/clients/kms"
)

type KMSEncryptionAdapter struct {
	engine *kms.EnvelopeEncryptionEngine
}

func NewKMSEncryptionAdapter(engine *kms.EnvelopeEncryptionEngine) ports.KmsEncryptionPort {
	return &KMSEncryptionAdapter{engine: engine}
}

func (a *KMSEncryptionAdapter) EncryptSecret(ctx context.Context, keyURI string, plaintext []byte, additionalData string) (string, error) {
	env, err := a.engine.EncryptSecret(ctx, keyURI, plaintext, additionalData)
	if err != nil {
		return "", err
	}
	return env.Ciphertext, nil
}
