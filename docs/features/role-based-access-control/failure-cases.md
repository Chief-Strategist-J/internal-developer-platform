# RBAC Failure Modes & Recovery Strategies

| Failure Mode | Root Cause | Impact | Automated Recovery / Mitigation | Severity |
| :--- | :--- | :--- | :--- | :--- |
| **Token Expired** | Client request carries timestamp past `exp` claim. | HTTP 401 Unauthorized returned to client. | Client refreshes token with IdP / OAuth refresh flow. | P3 |
| **Tampered / Invalid Signature** | JWT signature fails cryptographic HMAC/RSA validation. | HTTP 401 Unauthorized. Security event logged. | Immediately reject. Record suspicious incident in audit trail. | P1 |
| **Revoked Session / Blacklisted Subject** | Admin invoked emergency session kill; key found in Redis revocation store. | HTTP 401 Unauthorized on subsequent calls. | Client forced to re-authenticate against IdP. | P1 |
| **Redis Revocation Store Offline** | `idp-cache-1` unreachable / network partition. | Revocation checks fail. | Fail-secure posture: Reject privileged mutations or fallback to local in-memory blacklist while logging alerting metrics. | P1 |
| **Cross-Tenant Access Violation** | User authenticated in `tenant-alpha` attempts to read/write in `tenant-bravo`. | HTTP 403 Forbidden. Access blocked at domain boundary. | Hard stop by domain authorization rules; emit security audit event with caller identity. | P1 |
| **Insufficient Action Permission** | Role possesses `read` but attempts `write` or `delete`. | HTTP 403 Forbidden. | Inform client of missing required action claim. | P2 |
| **Privilege Escalation Attempt** | Non-admin caller passes simulated `admin` role without signed claims from trusted issuer. | Cryptographic verification fails -> HTTP 401. | Cryptographic signature protects claims integrity. | P0 |
