# Disaster Recovery Control Plane: Developer Configuration Guide

## 1. Overview & Open Standard Principles
This repository strictly adheres to the **Open Standards & Interoperability Policy** (`rules/open-standard/open.standard.md`).
- **Zero Proprietary Lock-In**: The Disaster Recovery (DR) control plane is vendor-neutral and portable.
- **Zero Hardcoded Identifiers or Credentials**: There are zero baked-in cloud project IDs, cluster names, service account keys, or tokens.
- **Dual Ingress Parity**: All capabilities are exposed with 1-to-1 operational parity between:
  1. **HTTP REST API** (`/api/v1/dr/*`)
  2. **Single-Binary CLI** (`go run ./src/main.go dr-ctl <subcommand>`)

---

## 2. Configuration Model (3-Tier Precedence)

Configuration values resolve in the following strict order of precedence (highest to lowest):

```
1. CLI Flags (`--base-url`, `--tenant`, `--token`, `--primary-region`)
   ↓ (falls back to)
2. Environment Variables (`DR_API_BASE_URL`, `GOOGLE_CLOUD_PROJECT`, `CLOUD_SQL_ADMIN_BASE_URL`)
   ↓ (falls back to)
3. Declarative Configuration Files (`config/default.yaml` validated against `config/env.schema`)
   ↓ (falls back to)
4. System Standard Constants (`src/shared/constants/system.constants.go`)
```

### Environment Variables Matrix

| Variable Name | Required? | Default / Fallback | Description |
|---|---|---|---|
| `DR_API_BASE_URL` | Optional | `http://127.0.0.1:8080` | URL of the running platform service instance |
| `DR_TENANT_ID` | Optional | `default` | Tenant identifier injected in `x-tenant-id` header |
| `DR_AUTH_TOKEN` | Optional | `""` (dev bypass) | Bearer JWT token for authenticated operations |
| `GOOGLE_CLOUD_PROJECT` | Optional | `""` (in-memory mode) | Target GCP Project ID for live Cloud SQL / Cloud DNS operations |
| `CLOUD_SQL_ADMIN_BASE_URL` | Optional | `https://sqladmin.googleapis.com/sql/v1beta4` | Cloud SQL REST endpoint (mockable for local test servers) |
| `CLOUD_DNS_BASE_URL` | Optional | `https://dns.googleapis.com/dns/v1` | Cloud DNS REST endpoint (mockable for local test servers) |
| `GOOGLE_OAUTH_ACCESS_TOKEN`| Optional | Retrieved via `gcloud` CLI | Explicit OAuth token override |

---

## 3. Quickstart for New Developers

### Step 1: Copy Environment Template
```bash
cp .env.example .env
```
Open `.env` and verify the settings. Notice that all credentials use generic, open placeholders (e.g. `GOOGLE_CLOUD_PROJECT=your-gcp-project-id`).

### Step 2: Start the Local Development Platform
Run the platform server via the unified single binary:
```bash
go run ./src/main.go
```
The server will boot listening on:
- Public HTTP API: `http://127.0.0.1:8080`
- Admin & Probes: `http://0.0.0.0:8081` (`/readyz`, `/livez`)

### Step 3: Run Disaster Recovery Commands
You can interact with the DR control plane through either the **CLI** or the **HTTP API**:

#### Option A: Single-Binary CLI
```bash
# Print available subcommands
go run ./src/main.go dr-ctl help

# 1. Create a new tier-governed DR plan
go run ./src/main.go dr-ctl plan:create \
  --plan core-pay-dr \
  --name "Core Payment DR" \
  --tier TIER_1 \
  --primary-region us-central1 \
  --standby-region us-east4 \
  --token "$JWT"

# 2. Query real-time replication lag
go run ./src/main.go dr-ctl plan:rpo --plan core-pay-dr

# 3. Run a simulated sandbox drill
go run ./src/main.go dr-ctl drill:run --plan core-pay-dr --scope ZONE
```

#### Option B: Developer Shell Runner (`scripts/dr/dr-ctl.sh`)
```bash
# Set your environment
export DR_API_BASE_URL="http://127.0.0.1:8080"
export DR_TENANT_ID="fintech-team"
export DR_AUTH_TOKEN="$JWT"

# Execute commands
./scripts/dr/dr-ctl.sh plan:check core-pay-dr
./scripts/dr/dr-ctl.sh drill:run core-pay-dr
```

#### Option C: Native HTTP REST API (`curl`)
```bash
# Create plan
curl -s -X POST "http://127.0.0.1:8080/api/v1/dr/plans" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $JWT" \
  -H "x-tenant-id: fintech-team" \
  -d '{
    "planName": "core-pay-dr",
    "incidentTier": "TIER_1",
    "primaryRegion": "us-central1",
    "drRegion": "us-east4",
    "topology": "WARM_STANDBY",
    "targetRtoSeconds": 1800,
    "targetRpoSeconds": 300,
    "dataStoreType": "CLOUD_SQL",
    "secretSyncSource": "GOOGLE_SECRET_MANAGER",
    "primaryClusterId": "gke-primary-us-central1",
    "drClusterId": "gke-standby-us-east4",
    "idempotencyKey": "idemp-001"
  }'
```

---

## 4. Working Offline or Against Simulated Infrastructure
If you do not have an active Google Cloud account or service keys:
- Leave `GOOGLE_CLOUD_PROJECT` empty in your `.env`.
- The platform automatically operates in **in-memory simulation mode** (`CloudProviderDRAdapter`), allowing you to test full end-to-end failovers, synthetic drills, dual-person break-glass approvals, and RFC-compliant response envelopes without connecting to external cloud infrastructure.
- To point to local mock servers (e.g. `httptest.Server` or Prism), set:
  ```bash
  export CLOUD_SQL_ADMIN_BASE_URL="http://127.0.0.1:9099/sqladmin"
  export CLOUD_DNS_BASE_URL="http://127.0.0.1:9099/dns"
  ```

