# ADR: Platform Technology Stack & Multi-Runtime Service Architecture

- **Status**: Accepted
- **Deciders**: Lead Systems Architect & Platform Engineering Team
- **Date**: 2026-11-02

---

## 1. Context and Problem Statement

The Internal Developer Platform (IDP) requires an enterprise-grade, polyglot, CNCF-aligned infrastructure to support self-service provisioning, service catalogs, policy enforcement, distributed observability, and automated rollouts across multiple environments.

The platform must balance developer ergonomics (Backstage portal), control plane efficiency (Go controllers), high-throughput computing (Rust workers), low-latency execution (C++ engines), and operational resilience without creating vendor lock-in.

---

## 2. Decision Matrix: Evaluated Services & Technologies

| Layer | Selected Technology | Version | Wire Port | Key Responsibility | Why Chosen & Key Trade-offs |
|---|---|:---:|:---:|---|---|
| **Control Plane API** | **Go** | `1.27.3` | `8080` | Platform APIs, controllers, and Kubernetes operators | Low memory footprint, fast startup, standard for cloud-native operators. Strict hexagonal domain boundary enforced. |
| **Developer Portal** | **React + TypeScript (Backstage)** | `Node 26.13` (TS 5.6) | `7007` | Service catalog, software templates, TechDocs | Industry standard for platform engineering portals, extensive plugin ecosystem. |
| **High-Throughput Worker** | **Rust** | `1.99.1` | `9000` | Asynchronous worker processing, queue processing | Zero-cost abstractions, memory safety without GC pauses. |
| **Compute Engine** | **C++ (CMake/GCC)** | `16.2.0` | `9090` | Ultra-low-latency execution engine | Maximum compute and memory control for critical platform algorithms. |
| **API Gateway & Ingress** | **Envoy (via Istio) / Caddy** | Caddy `2.9` / Envoy `1.31` | `80`, `443`, `8080` | Edge routing, TLS termination, W3C trace ingestion | Caddy for lightweight local development; Envoy/Istio for production mTLS and L7 routing. |
| **Container Engine** | **Docker / BuildKit** | `29.8` | N/A | Multi-stage, non-root image compilation | Standardized reproducible builds; enforced non-root (`10001:10001`) and `tini` PID 1 supervisors. |
| **Orchestration** | **Kubernetes (GKE)** | `1.30+` | `443` | Container scheduling, autoscaling, workload isolation | Industry standard; private clusters with Shielded VMs and Workload Identity. |
| **Local Development** | **Kind / K3d / Docker Compose** | Compose `v5.5` | N/A | Rapid local feedback loop | Fast iteration with mock backing services and live file watches. |
| **Infrastructure as Code** | **Terraform** | `1.17.1` (Google `~> 8.9`) | N/A | Cloud VPC, database, cache, and KMS provisioning | Declarative state management, cross-cloud capability, modular directory structure. |
| **K8s Packaging** | **Helm** | `v3` | N/A | Application packaging, parameterization, OCI push | Canonical packaging format for Kubernetes deployments with Kyverno policy hooks. |
| **GitOps Engine** | **Argo CD** | `v2.12` | `443` | Declarative GitOps reconciliation | Eliminates manual `kubectl` access; reconciles Git state automatically. |
| **Progressive Delivery** | **Argo Rollouts** | `v1.7` | N/A | Canary and blue-green zero-downtime deployments | Native integration with Istio metrics for automated rollback on error rate spikes. |
| **Service Mesh** | **Istio** | `1.23` | `15021`, `15090` | Transparent mTLS, L7 traffic management, tracing | Zero-code mutual TLS encryption and traffic shadowing. |
| **Secret Authority** | **HashiCorp Vault** | `1.17` | `8200` | Centralized KMS and secret lifecycle management | Enterprise HSM integration, dynamic secret leasing. |
| **Secret Synchronization** | **External Secrets Operator** | `v0.10` | N/A | Synchronizes Vault/GCP secrets into K8s Secrets | Decouples application code from secret vault SDKs. |
| **Policy Engine** | **Kyverno** | `1.12` | N/A | Kubernetes Pod Security & admission enforcement | Native YAML policy rules; avoids Rego learning curve while blocking root execution. |
| **Identity & SSO** | **Keycloak** | `25.0` | `8081` | OIDC, SAML, developer SSO authentication | Battle-tested open-source identity provider with fine-grained RBAC. |
| **Container Registry** | **Harbor / GHCR** | OCI Compliant | `443` | Private container image and Helm chart storage | Built-in vulnerability scanning, replication, and artifact signing verification. |
| **Artifact Security** | **Trivy** | Latest | N/A | Container and filesystem vulnerability scanning | Fast scanning integrated into release and packaging gates. |
| **Supply Chain Security**| **Cosign + SBOM** | Latest | N/A | Cryptographic image signing and provenance | Guarantees only signed container digests deploy into production. |
| **Distributed Tracing** | **Grafana Tempo** | `2.6.0` | `3200` | Distributed trace storage and query | High-efficiency object-storage backend with native Prometheus metrics generator. |
| **Centralized Logging** | **Grafana Loki** | `3.1.1` | `3100` | Log aggregation and indexing | Indexes metadata labels rather than full text, lowering storage costs. |
| **Metrics Engine** | **Prometheus** | `2.54.1` | `9090` | Time-series metrics collection | Native PromQL support, scrapes OTel collector and Tempo RED metrics. |
| **Visualization & Access**| **Grafana** | `11.2.0` | `3000` | Unified platform monitoring dashboards | Unifies traces (Tempo), logs (Loki), and metrics (Prometheus) with cross-linking. |
| **Telemetry Collector** | **OpenTelemetry Collector** | `0.108.0` | `4317`, `4318`, `8889` | Ingestion, processing, and batching gateway | Vendor-neutral pipeline decoupling applications from backend telemetry databases. |
| **Relational Database** | **PostgreSQL** | `18.6` | `5432` | System-of-record relational persistence | ACID transactions, JSONB flexibility, composite foreign key support. |
| **In-Memory Cache** | **Redis (Valkey)** | `8.10.3` | `6379` | Session storage, rate limiting, distributed locking | Sub-millisecond latency, RESP3 protocol, LRU eviction. |

---

## 3. End-to-End Tracing & Telemetry Architecture

Every execution flow across the platform is bound within an end-to-end W3C Trace Context space.

### 3.1 Trace Context Ingestion & Propagation
1. **Ingress Extraction**: Edge proxies (Envoy or Caddy) ingest the inbound W3C `traceparent` header (format: `00-{traceId}-{spanId}-{flags}`) or generate a compliant 128-bit UUIDv7 trace identifier if absent.
2. **Context Binding**: The trace identifier is injected into thread-local/context objects across all microservice requests.
3. **Internal Spans**:
   - `platform-api` (Go) initializes a Root Span via OpenTelemetry Go SDK.
   - Child spans are spawned at each execution boundary (`Router` -> `Handler` -> `Service` -> `Port`).
   - All outgoing requests to database adapters, the Rust worker (via gRPC metadata), and the C++ engine carry the propagated context.
4. **OTLP Export**: Traces are delivered asynchronously via OTLP gRPC (`0.0.0.0:4317`) or HTTP (`0.0.0.0:4318`) to the centralized OpenTelemetry Collector.
5. **Tempo Storage**: The OTel Collector flushes batched spans (`send_batch_size: 256`, `timeout: 1s`) to Grafana Tempo (`tempo:4317`).
6. **RED Metrics Generation**: Tempo's `metrics_generator` derives Rate, Errors, and Duration metrics from spans and streams them into Prometheus (`http://prometheus:9090/api/v1/write`) with exemplars.

---

## 4. Database & Stateful Configuration Architecture

### 4.1 PostgreSQL Database Configuration
- **Engine**: PostgreSQL 18.6 (Local Compose) / Cloud SQL PostgreSQL 18 (GCP Production).
- **Network Isolation**: Zero public IP exposure. Bound strictly to `backend` internal network (Local) or Private Service Access VPC Peering (GCP).
- **Security & Secrets**:
  - Root password never injected via plaintext environment variables.
  - Runtime container reads secret file: `POSTGRES_PASSWORD_FILE: /run/secrets/db_password`.
  - GCP Cloud SQL provisions credentials via Google Secret Manager and mounts them into GKE pods using `ExternalSecrets`.
- **Database Topology**:
  - Connection Pool: Dedicated pool with `max_connections = 100`, `idle_timeout = 10m`.
  - Concurrency Safety: Optimistic locking via `version BIGINT NOT NULL` on all mutable tables.
  - Multi-Tenant Composite Keys: Tenant partitioning leads every primary and foreign key (`UNIQUE(tenant_id, id)`).

### 4.2 Redis Cache Configuration
- **Engine**: Redis 8.10.3 / Memorystore Redis 7.0.
- **Port**: `6379`.
- **Memory & Eviction Policy**:
  - Memory clamp: `--maxmemory 200mb` (Local) / `1GB` (Cloud).
  - Eviction: `--maxmemory-policy allkeys-lru` (Least Recently Used eviction without crashing on memory ceiling).
  - Persistence: RDB snapshots disabled for pure cache tiers (`--save ""`).

---

## 5. Grafana Access Point & Unified Telemetry Integration

- **Access Point URL**: `http://127.0.0.1:3000` (Local Dev) / `https://grafana.internal.company.com` (Production Ingress).
- **Default Credentials**: `admin` / `admin` (Provisioned locally via compose secrets).
- **Data Source Linkage Matrix**:
  - **Prometheus (`http://prometheus:9090`)**: Default metrics datasource with Exemplars enabled linking to Tempo.
  - **Tempo (`http://tempo:3200`)**: UID `tempo`. Configured with `nodeGraph: enabled: true` and `serviceMap: datasourceUid: prometheus`.
  - **Loki (`http://loki:3100`)**: UID `loki`. Configured with `derivedFields` parsing `traceId: ([0-9a-f]{32})` and linking directly to `$${__value.raw}` in Tempo.

---

## 6. Environment & Configuration Manifests

Per Rule 00 & Rule 03, all environment configurations are decoupled from code and validated via strict schemas:

### 6.1 Environment Variable Taxonomy
| Variable | Environment | Default Value | Description |
|---|:---:|---|---|
| `APP_ENV` | All | `production` / `dev` | Application runtime environment tier |
| `PORT` | All | `8080` | Core Go API HTTP listening port |
| `METRICS_PORT` | All | `9090` | Prometheus metrics scraping port |
| `DB_HOST` | All | `db` (Compose) / `10.10.0.X` (GCP) | PostgreSQL host address |
| `DB_PORT` | All | `5432` | PostgreSQL port |
| `DB_NAME` | All | `idp_db` | Application relational database name |
| `DB_PASSWORD_FILE` | All | `/run/secrets/db_password` | Filepath to mounted database password |
| `CACHE_HOST` | All | `cache` (Compose) / `10.10.0.Y` (GCP)| Redis cache host address |
| `CACHE_PORT` | All | `6379` | Redis cache port |
| `OTEL_EXPORTER_OTLP_ENDPOINT`| All | `http://otel-collector:4317` | OTLP gRPC telemetry endpoint |
| `LOG_LEVEL` | All | `info` (Prod) / `debug` (Dev) | Structured logging severity threshold |
| `NODE_OPTIONS` | Portal | `--max-old-space-size=384` | Node.js memory ceiling (75% rule) |
