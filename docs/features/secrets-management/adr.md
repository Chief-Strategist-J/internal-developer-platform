# Architecture Decision Records (ADRs): Secrets Management & Cryptography

## ADR-001: Application-Layer CMEK Envelope Encryption (AES-256-GCM DEK + Cloud KMS KEK)
- **Status**: Accepted
- **Context**: Government compliance mandates Customer-Managed Encryption Keys (CMEK) and prohibits storing plaintext secrets in memory or persistent databases, while direct KMS API encryption for every secret read/write exceeds cloud API quotas and introduces prohibitive latency.
- **Decision**: Implement a two-tier envelope encryption engine (`src/infra/clients/kms/`):
  1. Generate an ephemeral 256-bit AES Data Encryption Key (DEK) for each payload.
  2. Encrypt the secret payload locally using AES-256-GCM with a random 96-bit nonce.
  3. Wrap the DEK with Google Cloud KMS Key Encryption Key (KEK).
  4. Persist the ciphertext alongside the encrypted DEK and authentication tag in a structured binary envelope.
- **Consequences**: Satisfies government CMEK custody requirements; reduces Cloud KMS API roundtrips; provides sub-millisecond encryption and decryption of workload secrets.

---

## ADR-002: Automated Dual-Issuer PKI and Certificate Lifecycle via `cert-manager.io/v1`
- **Status**: Accepted
- **Context**: Hand-rolling private CA keys, generating raw OpenSSL certificates in Go, and storing private keys in application containers creates key leakage vulnerabilities and manual rotation burdens.
- **Decision**: Implement declarative Certificate and Issuer manifest generation (`src/infra/clients/certificates/`) targeting `cert-manager.io/v1`:
  1. Support dual issuer hierarchies: Public ACME (Let's Encrypt / Google Trust Services) for public ingress and Private CA (Vault / ClusterIssuer) for internal mTLS.
  2. Configure automated renewal at 66% of certificate validity lifetime (`renewBefore: 720h`).
  3. Enforce 4096-bit RSA or ECDSA P-256/P-384 key algorithms.
- **Consequences**: Hand-rolled OpenSSL scripts are eliminated; certificates rotate automatically without pod downtime; Private CA root keys remain securely inside the dedicated PKI infrastructure.

