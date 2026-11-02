# Distributed Tracing — Business Logic Decision Trees

## 1. Architectural Overview & Tracing Pipeline Decision Tree

```
DISTRIBUTED TRACING ENGINE (SPAN PROPAGATION & SAMPLING DECISION TREE)
│
├── 1. CONTEXT PROPAGATION & INGRESS INGESTION TIER [P0]
│   └── Real-World: Mobile app calls API gateway; request spans 8 downstream microservices
│       └── Trigger: Ingress HTTP / gRPC Request Received
│           ├── Check: Does Request contain W3C Trace Context headers (`traceparent`, `tracestate`)?
│           │   ├── YES ──> Extract `trace_id` and parent `span_id` (Preserve Distributed Context)
│           │   └── NO ──> Action: Generate New Root `trace_id` (UUID / 128-bit random hex)
│           ├── Action: Create Root Span (`server` span kind)
│           └── Inject: Forward `traceparent` header to all outgoing downstream HTTP/gRPC calls
│           │
├── 2. SAMPLING STRATEGY & TAIL-BASED DECISION TIER [P0 / P1]
│   └── Real-World: System handles 50,000 requests/sec; saving 100% of traces would overwhelm storage
│       └── Evaluation: Multi-Stage Sampling Engine
│           ├── HEAD-BASED SAMPLING (API Gateway):
│           │   ├── Rule 1: High-Priority Routes (`/checkout`, `/payment`) ──> Sample Rate: 100%
│           │   ├── Rule 2: Synthetic Health Probes (`/healthz`) ──> Sample Rate: 0.1%
│           │   └── Rule 3: Standard Traffic ──> Deterministic Probabilistic Sample (e.g. 1%)
│           ├── TAIL-BASED SAMPLING (OpenTelemetry Collector Buffer):
│           │   ├── Action: Hold Spans in Memory Ring Buffer for 30 seconds
│           │   ├── Check: Did ANY Span in Trace encounter HTTP 5xx or Error Status?
│           │   │   ├── YES ──> [FORCE RETAIN: 100% Sampling for Error Traces — Ingest entire trace]
│           │   │   └── NO ──> Check: Did Trace Latency exceed P99 threshold (> 1000ms)?
│           │   │       ├── YES ──> [FORCE RETAIN: 100% Sampling for High-Latency Traces]
│           │   │       └── NO ──> Apply Standard Probabilistic Sample (Discard non-sampled traces)
│           │   └── State: SAMPLING_DECISION_FINALIZED
│           │
├── 3. SPAN ENRICHMENT & ATTRIBUTE REDACTION TIER [P0]
│   └── Real-World: Developer records database query and HTTP parameters in span attributes
│       └── Evaluation: Sanitization & Metadata Injection
│           ├── Action: Inject Platform Metadata (`k8s.pod.name`, `k8s.namespace.name`, `host.id`, `service.version`)
│           ├── Action: SQL Sanitizer (Normalize `SELECT * FROM users WHERE id = 123` to `WHERE id = ?`)
│           ├── Check: Does Span Attribute contain Raw Authorization Headers or PII?
│           │   ├── YES ──> [REDACT: Strip sensitive attribute values]
│           │   └── NO ──> Attributes Clean
│           └── State: SPAN_READY_FOR_EXPORT
│
└── 4. STORAGE & SERVICE GRAPH GENERATION TIER [P1]
    └── Real-World: Persisting spans to Tempo / Jaeger and generating live service dependency map
        └── Evaluation: Export Pipeline (OTLP over gRPC)
            ├── Action: Batch spans (1000 spans or 5 seconds)
            ├── Action: Write to Distributed Trace Store (Tempo / ClickHouse backend)
            ├── Action: Feed Trace Spans into Real-Time Service Graph Generator
            └── State: TRACE_SEARCHABLE (Available for developer root-cause analysis)
```

---

## 2. Invariants, Real-Life Rationale & Enforcement Trees

### Invariant P0-01: W3C Context Propagation Across Network Boundaries
- Real-Life Incident: A microservice used a proprietary tracing library that dropped standard headers; downstream spans appeared as disconnected orphan traces, blinding on-call engineers.
- Rationale: All services must adhere to W3C Trace Context specifications (`traceparent`), ensuring seamless end-to-end trace stitching across heterogeneous languages and frameworks.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: W3C TRACE PROPAGATION (P0-01)
Trigger: Outgoing RPC / HTTP Call
├── Check: Is `traceparent` header present in outgoing request headers?
│   ├── NO ──> [INJECT: OpenTelemetry SDK auto-injects current active trace context]
│   └── YES ──> Context Preserved
```

### Invariant P0-02: 100% Retention for Error and Latency Anomalies (Tail Sampling)
- Real-Life Incident: A rare concurrency deadlock affected 1 in 10,000 payments; a 1% head-based sampling strategy failed to capture any traces of the failure for 3 months.
- Rationale: High-throughput platforms must use tail-based sampling to guarantee that any trace containing an error or severe latency degradation is retained regardless of base sample rates.
- Enforcement Logic:
```
INVARIANT ENFORCEMENT: TAIL ERROR PRESERVATION (P0-02)
Trigger: Trace Completion in Collector Buffer
├── Check: Trace Contains Error (`status.code == ERROR`) OR Latency > 1000ms?
│   ├── YES ──> [FORCE RETAIN: Override sample rate to 100%]
│   └── NO ──> Follow base probabilistic sampling
```

---

## 3. Real-Life Edge Cases & Decision Trees

### Case 01: High-Frequency Clock Drift Across Distributed Nodes
- Real-Life Incident: Server clock skew caused child spans to appear to finish 50ms before their parent span started, breaking trace visualization tools.
- Decision Tree:
```
EDGE CASE 01: DISTRIBUTED CLOCK DRIFT
Trigger: Trace Stitching in UI Explorer
├── Check: Child Span Start Time < Parent Span Start Time?
│   ├── YES ──> [TIME SKEW ADJUSTMENT: Align child span within parent boundaries based on network RTT]
│   └── NO ──> Standard rendering
```

---

## 4. Phase-Gated Engineering Delivery Roadmap Tree

```
DISTRIBUTED TRACING ROADMAP
│
├── PHASE 0: OPENTELEMETRY SDK INJECTION & W3C HEADERS (P0 - WEEKS 1-3)
│   ├── Golden path templates pre-configured with OpenTelemetry instrumentation
│   ├── W3C traceparent context propagation across HTTP and gRPC frameworks
│   ├── OpenTelemetry Collector daemon deployment across all clusters
│   └── Deliverable: End-to-end trace propagation across all platform services
│
├── PHASE 1: TAIL-BASED SAMPLING & ERROR RETENTION (P1 - WEEKS 4-6)
│   ├── OpenTelemetry Collector tail-sampling configuration
│   ├── 100% retention for all HTTP 5xx errors and P99 latency anomalies
│   └── Deliverable: High-value diagnostic trace capture with 70% storage reduction
│
├── PHASE 2: TEMPO / JAEGER INTEGRATION & SERVICE GRAPHS (P2 - WEEKS 7-9)
│   ├── High-scale Grafana Tempo / ClickHouse trace storage integration
│   ├── Automated real-time service topology and dependency graph rendering
│   └── Deliverable: Visual trace explorer embedded in Developer Portal service pages
│
└── PHASE 3: DISTRIBUTED CRITICAL-PATH LATENCY ANALYSIS (P3 - WEEKS 10-12)
    ├── Automated bottleneck identification (Highlighting slow database queries/lock waits)
    └── Deliverable: Autonomous performance profiling pointing directly to offending spans
```
