package envelope

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"sync"

	"internal-developer-platform/src/shared/errors"
)

type MockKMSClient struct {
	mu   sync.RWMutex
	keys map[string][]byte
}

func NewMockKMSClient() *MockKMSClient {
	return &MockKMSClient{
		keys: make(map[string][]byte),
	}
}

func (m *MockKMSClient) RegisterKey(keyURI string, key []byte) error {
	if len(key) != 32 {
		return errors.NewValidationError("kms key must be exactly 32 bytes for aes-256")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys[keyURI] = key
	return nil
}

func (m *MockKMSClient) Encrypt(ctx context.Context, keyURI string, plaintext []byte) ([]byte, error) {
	m.mu.RLock()
	key, exists := m.keys[keyURI]
	m.mu.RUnlock()

	if !exists {
		return nil, errors.NewNotFoundError("kms key uri not found: " + keyURI)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.NewConflictError("failed to initialize aes cipher: " + err.Error())
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.NewConflictError("failed to initialize gcm: " + err.Error())
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, errors.NewConflictError("failed to generate nonce: " + err.Error())
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, []byte(keyURI))
	return ciphertext, nil
}

func (m *MockKMSClient) Decrypt(ctx context.Context, keyURI string, ciphertext []byte) ([]byte, error) {
	m.mu.RLock()
	key, exists := m.keys[keyURI]
	m.mu.RUnlock()

	if !exists {
		return nil, errors.NewNotFoundError("kms key uri not found: " + keyURI)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.NewConflictError("failed to initialize aes cipher: " + err.Error())
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.NewConflictError("failed to initialize gcm: " + err.Error())
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, errors.NewValidationError("ciphertext too short")
	}

	nonce, actualCiphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, actualCiphertext, []byte(keyURI))
	if err != nil {
		return nil, errors.NewValidationError("kms decryption failed or authentication tag mismatch")
	}

	return plaintext, nil
}

