package kms

import (
	"context"
	"crypto/rand"
	"io"
	"testing"
)

func TestEnvelopeEncryptionRoundtrip(t *testing.T) {
	mockKMS := NewMockKMSClient()
	masterKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, masterKey); err != nil {
		t.Fatalf("failed to generate master key: %v", err)
	}

	keyURI := "projects/idp-sec/locations/global/keyRings/tenant-ring/cryptoKeys/kek-001"
	if err := mockKMS.RegisterKey(keyURI, masterKey); err != nil {
		t.Fatalf("failed to register key: %v", err)
	}

	engine := NewEnvelopeEncryptionEngine(mockKMS)
	ctx := context.Background()

	secretPayload := []byte("postgres://db_user:s3cr3t_p@ssw0rd@postgres.internal:5432/orders")
	aad := "tenant-live:orders-db"

	env, err := engine.EncryptSecret(ctx, keyURI, secretPayload, aad)
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	if env.Algorithm != AlgorithmAES256GCM {
		t.Fatalf("expected algorithm %s, got %s", AlgorithmAES256GCM, env.Algorithm)
	}
	if env.KeyURI != keyURI {
		t.Fatalf("expected keyURI %s, got %s", keyURI, env.KeyURI)
	}

	decrypted, err := engine.DecryptSecret(ctx, env)
	if err != nil {
		t.Fatalf("decryption failed: %v", err)
	}

	if string(decrypted.Plaintext) != string(secretPayload) {
		t.Fatalf("decrypted plaintext does not match original secret")
	}
}

func TestEnvelopeEncryptionTamperDetection(t *testing.T) {
	mockKMS := NewMockKMSClient()
	masterKey := make([]byte, 32)
	_, _ = io.ReadFull(rand.Reader, masterKey)
	keyURI := "projects/idp-sec/locations/global/keyRings/tenant-ring/cryptoKeys/kek-001"
	_ = mockKMS.RegisterKey(keyURI, masterKey)

	engine := NewEnvelopeEncryptionEngine(mockKMS)
	ctx := context.Background()

	env, err := engine.EncryptSecret(ctx, keyURI, []byte("sensitive-api-token"), "tenant-1")
	if err != nil {
		t.Fatalf("encryption failed: %v", err)
	}

	tamperedEnv := *env
	tamperedEnv.Ciphertext = "AAAA" + tamperedEnv.Ciphertext[4:]

	_, err = engine.DecryptSecret(ctx, &tamperedEnv)
	if err == nil {
		t.Fatalf("expected tamper detection to reject modified ciphertext")
	}

	tamperedAAD := *env
	tamperedAAD.AdditionalData = "tenant-hacked"
	_, err = engine.DecryptSecret(ctx, &tamperedAAD)
	if err == nil {
		t.Fatalf("expected tamper detection to reject modified additional data")
	}
}

func TestEnvelopeEncryptionValidationErrors(t *testing.T) {
	mockKMS := NewMockKMSClient()
	engine := NewEnvelopeEncryptionEngine(mockKMS)
	ctx := context.Background()

	_, err := engine.EncryptSecret(ctx, "", []byte("test"), "")
	if err == nil {
		t.Fatalf("expected error for empty keyURI")
	}

	_, err = engine.EncryptSecret(ctx, "key-1", nil, "")
	if err == nil {
		t.Fatalf("expected error for empty plaintext")
	}

	_, err = engine.DecryptSecret(ctx, nil)
	if err == nil {
		t.Fatalf("expected error for nil envelope")
	}
}
