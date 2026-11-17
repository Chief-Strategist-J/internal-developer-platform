package kms

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
	"time"

	"internal-developer-platform/src/shared/errors"
)

type EnvelopeEncryptionEngine struct {
	kmsClient KMSClientPort
}

func NewEnvelopeEncryptionEngine(kmsClient KMSClientPort) *EnvelopeEncryptionEngine {
	return &EnvelopeEncryptionEngine{
		kmsClient: kmsClient,
	}
}

func (e *EnvelopeEncryptionEngine) EncryptSecret(ctx context.Context, keyURI string, plaintext []byte, additionalData string) (*EncryptedEnvelope, error) {
	if len(plaintext) == 0 {
		return nil, errors.NewValidationError("plaintext secret cannot be empty")
	}
	if keyURI == "" {
		return nil, errors.NewValidationError("kms key uri cannot be empty")
	}

	dek := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, errors.NewConflictError("failed to generate cryptographic dek: " + err.Error())
	}

	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, errors.NewConflictError("failed to create cipher block: " + err.Error())
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.NewConflictError("failed to create gcm: " + err.Error())
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, errors.NewConflictError("failed to generate nonce: " + err.Error())
	}

	var aad []byte
	if additionalData != "" {
		aad = []byte(additionalData)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)

	encryptedDEK, err := e.kmsClient.Encrypt(ctx, keyURI, dek)
	if err != nil {
		return nil, errors.NewDownstreamUnavailableError("kms dek encryption failed: " + err.Error())
	}

	return &EncryptedEnvelope{
		Algorithm:      AlgorithmAES256GCM,
		KeyURI:         keyURI,
		EncryptedDEK:   base64.StdEncoding.EncodeToString(encryptedDEK),
		Nonce:          base64.StdEncoding.EncodeToString(nonce),
		Ciphertext:     base64.StdEncoding.EncodeToString(ciphertext),
		AdditionalData: additionalData,
		CreatedAt:      time.Now().UTC(),
	}, nil
}

func (e *EnvelopeEncryptionEngine) DecryptSecret(ctx context.Context, env *EncryptedEnvelope) (*DecryptedPayload, error) {
	if env == nil {
		return nil, errors.NewValidationError("encrypted envelope cannot be nil")
	}
	if env.KeyURI == "" {
		return nil, errors.NewValidationError("envelope key uri cannot be empty")
	}

	encryptedDEK, err := base64.StdEncoding.DecodeString(env.EncryptedDEK)
	if err != nil {
		return nil, errors.NewValidationError("malformed base64 encrypted dek")
	}

	nonce, err := base64.StdEncoding.DecodeString(env.Nonce)
	if err != nil {
		return nil, errors.NewValidationError("malformed base64 nonce")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return nil, errors.NewValidationError("malformed base64 ciphertext")
	}

	dek, err := e.kmsClient.Decrypt(ctx, env.KeyURI, encryptedDEK)
	if err != nil {
		return nil, errors.NewDownstreamUnavailableError("kms dek decryption failed: " + err.Error())
	}

	if len(dek) != 32 {
		return nil, errors.NewValidationError("decrypted dek has invalid length")
	}

	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, errors.NewConflictError("failed to create cipher block from dek: " + err.Error())
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.NewConflictError("failed to create gcm from dek: " + err.Error())
	}

	if len(nonce) != gcm.NonceSize() {
		return nil, errors.NewValidationError("invalid nonce size")
	}

	var aad []byte
	if env.AdditionalData != "" {
		aad = []byte(env.AdditionalData)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, errors.NewValidationError("envelope payload authentication or decryption failed")
	}

	return &DecryptedPayload{
		Plaintext: plaintext,
		KeyURI:    env.KeyURI,
	}, nil
}

