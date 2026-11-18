# Architecture & Operational Diagrams: Secrets Management & Cryptography

This document details the runtime architecture of application-layer Customer-Managed Encryption Key (CMEK) envelope encryption and declarative PKI certificate lifecycles via `cert-manager.io/v1`.

---

## 1. CMEK Envelope Encryption & Decryption Runtime Execution ASCII Tree

The following ASCII tree outlines the two-tier envelope encryption process (AES-256-GCM + Google Cloud KMS) executed by `src/infra/clients/kms/`:

```text
[Secret Encryption Pipeline] : EnvelopeEncryptionEngine.Encrypt(ctx, plaintext)
  │
  ├── 1. Distributed Tracing & Span Setup
  │     ├── Open OTel Child Span: "security.kms.envelope_encrypt"
  │     ├── Set Attributes: key_ring, crypto_key, algorithm ("AES-256-GCM")
  │     └── Defer span.End()
  │
  ├── 2. Ephemeral Data Encryption Key (DEK) Generation
  │     ├── Read 32 cryptographically secure random bytes from crypto/rand
  │     ├── RawDEK = crypto/rand.Read(32 bytes)
  │     └── Ephemeral DEK exists solely in transient memory
  │
  ├── 3. Local Application-Layer AES-256-GCM Encryption
  │     ├── Generate random 12-byte initialization vector (Nonce)
  │     ├── Initialize AES-256 cipher block using RawDEK
  │     ├── Initialize GCM AEAD (Authenticated Encryption with Associated Data)
  │     ├── Encrypt Payload: Ciphertext = GCM.Seal(nil, Nonce, Plaintext, AdditionalAuthData)
  │     └── Note: AuthTag (16 bytes) is appended to Ciphertext by GCM
  │
  ├── 4. Cloud KMS Key Wrapping (KEK)
  │     ├── Call Cloud KMS Encrypt API with Key Encryption Key (KEK):
  │     │     ├── Request: kms.Encrypt(Name: KEK_Resource_Path, Plaintext: RawDEK)
  │     │     └── Response: EncryptedDEK (Ciphertext blob wrapped by KMS HSM)
  │     │
  │     └── Zeroize RawDEK in memory immediately after KMS wrapping
  │
  └── 5. Binary Envelope Packing
        ├── Assemble Structured Binary Envelope:
        │     ├── Magic Byte Header (4 bytes: "CMEK")
        │     ├── Key Version & Metadata (4 bytes)
        │     ├── Length of EncryptedDEK (2 bytes) + EncryptedDEK Payload
        │     ├── Nonce (12 bytes)
        │     └── Ciphertext + GCM Tag
        └── Return Base64-encoded structured envelope for persistent storage

─────────────────────────────────────────────────────────────────────────────

[Secret Decryption Pipeline] : EnvelopeEncryptionEngine.Decrypt(ctx, envelope)
  │
  ├── 1. Envelope Unpacking & Validation
  │     ├── Verify "CMEK" magic header and supported cipher version
  │     └── Extract: EncryptedDEK, Nonce, and Ciphertext
  │
  ├── 2. Cloud KMS Key Unwrapping
  │     ├── Call Cloud KMS Decrypt API:
  │     │     ├── Request: kms.Decrypt(Name: KEK_Resource_Path, Ciphertext: EncryptedDEK)
  │     │     └── Response: Recovered RawDEK (32 bytes)
  │     └── ⚠ On KMS Failure: RecordError(span, err) → Return 403 ForbiddenError
  │
  ├── 3. Local AES-256-GCM Decryption & Authenticated Tag Verification
  │     ├── Initialize GCM AEAD using unwrapped RawDEK
  │     ├── GCM.Open(nil, Nonce, Ciphertext, AdditionalAuthData)
  │     ├── ↳ If AuthTag Mismatch (Tampering / Bit Flip):
  │     │     ├── Panic / Fail-Closed: Return TamperDetectedError
  │     │     └── Zeroize all buffers
  │     │
  │     └── Zeroize RawDEK from memory
  │
  └── 4. Return Decrypted Plaintext Payload
```

---

## 2. CMEK Two-Tier Envelope Encryption Architecture

```mermaid
flowchart TD
    subgraph LocalApplication["Platform Application Layer"]
        PLAIN["Secret Plaintext<br/>(API Keys, DB Credentials)"]
        GEN_DEK["Generate Ephemeral 256-bit DEK<br/>(crypto/rand)"]
        AES_GCM["AES-256-GCM AEAD Engine<br/>(Nonce: 12 bytes, Tag: 16 bytes)"]
        CIPHER["Encrypted Payload Ciphertext"]
        ENV["Structured Binary Envelope<br/>(Header + Wrapped DEK + Nonce + Ciphertext)"]
    end

    subgraph CloudKMS["Google Cloud KMS (HSM Layer)"]
        KEK["Key Encryption Key (KEK)<br/>(FIPS 140-2 Level 3 HSM)"]
        WRAP["KMS Encrypt Operation"]
        WRAPPED_DEK["Wrapped DEK Blob"]
    end

    PLAIN --> AES_GCM
    GEN_DEK --> AES_GCM
    AES_GCM --> CIPHER

    GEN_DEK --> WRAP
    KEK --> WRAP
    WRAP --> WRAPPED_DEK

    WRAPPED_DEK --> ENV
    CIPHER --> ENV
```

---

## 3. Cert-Manager Dual-Issuer Automated PKI Lifecycle

```mermaid
sequenceDiagram
    autonumber
    participant Workload as Workload Engine
    participant CertClient as Certificate Client (src/infra/clients/certificates)
    participant K8s as Kubernetes API
    participant CertMgr as cert-manager Controller
    participant Issuer as ClusterIssuer (Private CA / ACME)
    participant Secret as Target K8s Secret (tls.crt, tls.key)

    Workload->>CertClient: GenerateCertificateManifest(spec)
    CertClient->>CertClient: Configure renewBefore: 720h (66% of lifetime)
    CertClient->>K8s: Submit Certificate & Issuer Custom Resources
    K8s-->>CertMgr: Informer Triggers Certificate Reconciliation

    CertMgr->>Issuer: Generate CertificateRequest (CSR)
    alt Private Internal mTLS
        Issuer->>Issuer: Sign via Private CA Root Key
    else Public Ingress
        Issuer->>Issuer: ACME HTTP-01 / DNS-01 Challenge
    end
    Issuer-->>CertMgr: Signed X.509 Certificate Chain

    CertMgr->>Secret: Write tls.crt, tls.key, ca.crt
    Secret-->>Workload: Pod Mounts TLS Secret

    loop Continuous Certificate Watch
        CertMgr->>Secret: Check Expiration Timestamp
        alt Certificate Reaches 66% Lifetime (renewBefore)
            CertMgr->>Issuer: Automatically Issue Renewal CSR
            Issuer-->>Secret: Overwrite with Rotated Key & Cert (Zero Downtime)
        end
    end
```
