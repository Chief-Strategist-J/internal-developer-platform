package envelope

import (
	"context"
)

type KMSClientPort interface {
	Encrypt(ctx context.Context, keyURI string, plaintext []byte) ([]byte, error)
	Decrypt(ctx context.Context, keyURI string, ciphertext []byte) ([]byte, error)
}
