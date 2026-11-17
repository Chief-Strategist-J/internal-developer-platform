package envelope

import (
	"time"
)

type EnvelopeAlgorithm string

const (
	AlgorithmAES256GCM EnvelopeAlgorithm = "AES-256-GCM-KMS-ENVELOPE"
)

type EncryptedEnvelope struct {
	Algorithm        EnvelopeAlgorithm `json:"algorithm"`
	KeyURI           string            `json:"key_uri"`
	EncryptedDEK     string            `json:"encrypted_dek"`
	Nonce            string            `json:"nonce"`
	Ciphertext       string            `json:"ciphertext"`
	AdditionalData   string            `json:"additional_data,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
}

type DecryptedPayload struct {
	Plaintext []byte
	KeyURI    string
}

