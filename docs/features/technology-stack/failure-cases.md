# Technology Stack Failure Modes, Edge Cases & Operational Mitigations

This document outlines critical failure scenarios across the platform's selected service technologies and the exact architectural mitigations enforced in code.

---

## 1. Multi-Runtime & Process Lifecycles

### Failure Case 1.1: Zombie Processes & Signal Swallowing in Containers
- **Symptom**: Containers take exactly 30–60s to terminate during rolling deployments, causing deployment hangs or orphaned processes.
- **Root Cause**: Node.js and Go processes running as PID 1 do not handle POSIX signals (`SIGTERM`, `SIGINT`) properly or reap orphaned child processes.
- **Architectural Mitigation**:
  - Enforce `/usr/bin/tini` as the container `ENTRYPOINT` across all production Dockerfiles (`Dockerfile`, `Dockerfile.rust`, `Dockerfile.cpp`, `Dockerfile.base`).
  - Configure `STOPSIGNAL SIGTERM` and explicit shutdown contexts in `src/main.go` using a 9-phase lifecycle.

### Failure Case 1.2: Host Memory Exhaustion (Kernel OOM Killer)
- **Symptom**: Unannounced container death (`Exit Code 137`) without application error logs.
- **Root Cause**: Node.js v8 garbage collector or Go GC allows in-memory allocations to hit the hard container cgroup limit.
- **Architectural Mitigation**:
  - Bound runtime heap size in container manifests: `NODE_OPTIONS="--max-old-space-size=384"` for a 512MB limit (75% rule).
  - Configure GKE pod resource limits with explicit `requests` and `limits` in Helm values and base deployments.

---

## 2. Ingress & Edge Proxying

### Failure Case 2.1: Intermittent 502 Bad Gateway on Reused Connections
- **Symptom**: Edge proxy logs `502 Bad Gateway` on a small percentage of idle HTTP/2 or keep-alive requests.
- **Root Cause**: The backend application server's idle timeout is shorter than or equal to the upstream load balancer's idle timeout, leading to race-condition connection closures.
- **Architectural Mitigation**:
  - Enforce Rule 06: Server idle timeout (`ReadTimeout: 75s`) must strictly exceed the upstream load balancer idle timeout (`60s`).

### Failure Case 2.2: Slowloris & Trickling Request Floods
- **Symptom**: Connection table exhaustion on the edge proxy while CPU and memory remain low.
- **Root Cause**: Slow clients trickling headers byte-by-byte without completing the HTTP request envelope.
- **Architectural Mitigation**:
  - Enforce `ReadHeaderTimeout: 10s` measured from the arrival of the first byte (never reset by subsequent bytes).
  - Cap maximum request body and header size in Caddy and Envoy configurations.

---

## 3. Database & Stateful Persistence

### Failure Case 3.1: Dual-Write Outbox Inconsistency
- **Symptom**: Entity is saved in PostgreSQL, but asynchronous worker never receives the corresponding event notification (or vice-versa).
- **Root Cause**: Writing to database and emitting a network event in separate non-atomic blocks.
- **Architectural Mitigation**:
  - Rule 02 & 05: All state mutations and outbox events must commit in the exact same relational transaction boundary (`UnitOfWork`).

### Failure Case 3.2: Concurrent Mutation Race Conditions
- **Symptom**: Two simultaneous requests overwrite each other's data (Lost Update problem).
- **Root Cause**: Unversioned updates relying on client-supplied data without optimistic locking.
- **Architectural Mitigation**:
  - Every mutable SQL table declares `version BIGINT NOT NULL`.
  - Mutating queries enforce `WHERE id = :id AND version = :version` and increment version atomically. Zero rows affected maps to `PreconditionFailedError` (HTTP 412).

---

## 4. Observability Pipeline

### Failure Case 4.1: Observability Black-Hole Under High Traffic
- **Symptom**: OTel Collector crashes or exhausts memory during load spikes, dropping traces and metrics.
- **Root Cause**: Unbounded memory buffering and unbatched exporter flushes.
- **Architectural Mitigation**:
  - OTel Collector configuration enforces memory limiter extension and `batch` processor (`send_batch_size: 256`, `timeout: 1s`).
  - Deployments define explicit `tmpfs` mounts and resource boundaries.

### Failure Case 4.2: High-Cardinality Metric Explosion
- **Symptom**: Prometheus server OOMs or queries crawl due to million-series metric explosions.
- **Root Cause**: Injecting unbounded dynamic values (e.g. `user_id`, `tenant_id`, `request_id`, or raw URLs `/api/v1/users/123`) into Prometheus metric labels.
- **Architectural Mitigation**:
  - Rule 06: Metric labels are strictly bounded to parameterized route patterns (`http.route = "/api/v1/users/{id}"`). IDs belong exclusively in structured logs and OpenTelemetry span attributes.
