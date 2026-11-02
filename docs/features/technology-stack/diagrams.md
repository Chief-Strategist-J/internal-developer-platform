# System Architecture, Distributed Tracing & Service Topography Diagrams

## 1. High-Level Service Interaction Architecture

```mermaid
flowchart TD
    subgraph Client Tier
        DevUser["Platform Developer / Web Browser"]
    end

    subgraph Ingress Tier
        EdgeProxy["Envoy / Caddy Gateway (Port 80/443/8080)"]
    end

    subgraph Application Tier
        Portal["Developer Portal (React + TypeScript :7007)"]
        ControlPlane["Platform API Core (Go :8080)"]
        RustWorker["Async Task Worker (Rust :9000)"]
        CppEngine["Low-Latency Engine (C++ :9090)"]
    end

    subgraph Stateful Storage Tier
        DB[("PostgreSQL 18.6 (:5432)")]
        Cache[("Redis Cache 8.10 (:6379)")]
    end

    subgraph Observability Pipeline
        OTelCol["OTel Collector (:4317/:4318)"]
        Tempo["Grafana Tempo (:3200)"]
        Loki["Grafana Loki (:3100)"]
        Prometheus["Prometheus (:9090)"]
        Grafana["Grafana UI (:3000)"]
    end

    DevUser -->|HTTPS| EdgeProxy
    EdgeProxy -->|UI Routes /| Portal
    EdgeProxy -->|API Routes /api/v1/*| ControlPlane

    ControlPlane -->|Read / Write| DB
    ControlPlane -->|Session / Rate Limit| Cache
    ControlPlane -->|gRPC / Internal Task| RustWorker
    ControlPlane -->|TCP / High Perf Compute| CppEngine

    ControlPlane -.->|OTLP Traces & Logs| OTelCol
    Portal -.->|Telemetry| OTelCol
    RustWorker -.->|Telemetry| OTelCol
    CppEngine -.->|Telemetry| OTelCol

    OTelCol -->|Spans| Tempo
    OTelCol -->|Logs| Loki
    OTelCol -->|Scraped Metrics| Prometheus

    Tempo -.->|Service Graph RED Metrics| Prometheus
    Grafana -->|Query Traces| Tempo
    Grafana -->|Query Logs| Loki
    Grafana -->|Query Metrics| Prometheus
```

---

## 2. Distributed Tracing & W3C Context Propagation Sequence

```mermaid
sequenceDiagram
    autonumber
    actor Developer
    participant Ingress as Envoy / Caddy Gateway (:8080)
    participant Core as Platform API Go (:8080)
    participant Rust as Rust Worker (:9000)
    participant DB as PostgreSQL 18.6 (:5432)
    participant OTel as OpenTelemetry Collector (:4318)
    participant Tempo as Grafana Tempo (:3200)
    participant Loki as Grafana Loki (:3100)
    participant Grafana as Grafana Dashboard (:3000)

    Developer->>Ingress: POST /api/v1/service-catalog
    Note over Ingress: Ingests or starts W3C traceparent<br/>(traceId: 4bf92f3577b34da6a3ce929d0e0e4736)
    
    Ingress->>Core: Forward request with traceparent
    Note over Core: Router matches path<br/>Handler starts Root Span<br/>Service executes UnitOfWork

    Core->>DB: SQL Query with trace comment
    DB-->>Core: Return rows
    
    Core->>Rust: gRPC async job with trace context metadata
    Rust-->>Core: ACK job accepted
    
    Core-->>Ingress: RFC Envelope (success, data, meta.traceId)
    Ingress-->>Developer: HTTP 200 OK + header X-Trace-Id

    par Asynchronous Telemetry Flush
        Core->>OTel: Export Spans & Structured Logs (with traceId)
        Rust->>OTel: Export Worker Child Spans
        OTel->>Tempo: Ingest Traces via OTLP gRPC
        OTel->>Loki: Ingest Logs with label traceId
    end

    Developer->>Grafana: Inspect logs in Loki
    Note over Grafana: Loki derivedFields detects traceId<br/>Renders direct clickable link
    Developer->>Grafana: Clicks TraceID link
    Grafana->>Tempo: Query full waterfall trace span by ID
```

---

## 3. Storage & Network Security Isolation Diagram

```mermaid
flowchart TD
    subgraph Public Internet / Untrusted Zone
        Client["External Developer / Ingress Traffic"]
    end

    subgraph DMZ / Frontend Network
        Caddy["Edge Proxy (Caddy / Envoy)"]
        PortalUI["Backstage React Portal"]
    end

    subgraph Internal Isolated Network [backend - internal: true]
        CoreAPI["Platform API Core (Go)"]
        RustSvc["Rust Async Worker"]
        CppSvc["C++ Low-Latency Engine"]
        
        subgraph Stateful Quarantined Stores
            Postgres["PostgreSQL 18.6 (No Host Port Binding)"]
            RedisStore["Redis 8.10 Cache (No Host Port Binding)"]
        end
    end

    subgraph Telemetry Fabric
        Collector["OTel Collector (4317/4318)"]
        TempoStore["Tempo"]
        LokiStore["Loki"]
        PromStore["Prometheus"]
        GrafanaUI["Grafana (Port 3000 Access Point)"]
    end

    Client -->|Port 80/443| Caddy
    Caddy -->|Internal Route| PortalUI
    Caddy -->|Internal Route| CoreAPI

    CoreAPI -->|Port 5432| Postgres
    CoreAPI -->|Port 6379| RedisStore
    CoreAPI -->|gRPC 9000| RustSvc
    CoreAPI -->|TCP 9090| CppSvc

    CoreAPI -.-> Collector
    RustSvc -.-> Collector
    Collector --> TempoStore
    Collector --> LokiStore
    Collector --> PromStore

    GrafanaUI --> TempoStore
    GrafanaUI --> LokiStore
    GrafanaUI --> PromStore
```
