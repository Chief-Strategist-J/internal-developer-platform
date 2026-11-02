# Visual Architectural Diagrams: Container Packaging and Lifecycle

## 1. Multi-Stage Build Lifecycle Pipeline

The following flowchart illustrates the segregation between heavy SDK compiler environments and minimal runtime packaging.

```mermaid
flowchart TD
    subgraph BuildKit_Engine ["BuildKit Build Stage (Heavy Toolchains)"]
        A1["Source Code (Go / TS / Rust / C++)"] --> B1["Builder Container"]
        B1 --> C1["golang:1.27 / rust:1.80 / gcc:14 / node:26"]
        C1 --> D1["Mount Package Cache (/go/pkg, cargo index, npm)"]
        D1 --> E1["Compile & Strip Artifacts (-s -w / Release)"]
    end

    subgraph Runtime_Stage ["Production Runtime Stage (Minimal Attack Surface)"]
        F1["Base OS: debian:bookworm-slim / node:slim"]
        F1 --> G1["Install ca-certificates & tini"]
        G1 --> H1["Create unprivileged user app:app (10001:10001)"]
        E1 --"COPY --from=builder --chown=10001:10001"--> I1["Inject Binary / dist"]
        H1 --> I1
        I1 --> J1["USER 10001:10001"]
        J1 --> K1["ENTRYPOINT: tini -- /app/service-bin"]
    end
```

---

## 2. Process Tree & Signal Propagation Lifecycle

The sequence diagram demonstrates how container init (`tini`) prevents zombie processes and guarantees clean socket draining during Kubernetes rolling updates.

```mermaid
sequenceDiagram
    autonumber
    actor K8s as Kubernetes Kubelet
    participant Tini as tini (PID 1)
    participant App as Service Process (PID 2)
    participant Port as Network Sockets (8080/9090)

    K8s->>Tini: SIGTERM (Termination Grace Period Begins)
    Tini->>App: Forward SIGTERM
    App->>Port: Close Public Listener (Stop Accepting Ingress)
    App->>Port: Drain Active In-Flight Requests (5s timeout)
    App->>App: Flush Telemetry Spans & DB Connections
    App-->>Tini: Process Exit (Code 0)
    Tini-->>K8s: Container Termination Complete
```

---

## 3. Storage Layer Distribution & Footprint Breakdown

```mermaid
xychart-beta
    title "Virtual Container Image Sizes Across Polyglot Runtimes (MB)"
    x-axis ["Go (platform-api)", "C++ (cpp-engine)", "Rust (rust-worker)", "TS (developer-portal)"]
    y-axis "Image Size in Megabytes" 0 --> 300
    bar [87.31, 80.93, 81.43, 263.45]
```
