# Failure Modes & Resilience Engineering: Container Runtimes & Multi-Stage Builds

## 1. Zombie Process Accumulation (PID 1 Reaper Failure)

### Context & Failure Mode
When an application process acts as PID 1 inside a Linux container, the Linux kernel assigns it special responsibilities: reaping orphan child processes that have terminated. If a child process is spawned and terminates while the parent does not explicitly call `wait()`/`waitpid()`, the process enters a `<defunct>` (zombie) state. Over time, the PID table of the container is exhausted, causing `fork: cannot allocate memory` errors despite ample free RAM.

### Mitigation Invariant
- Every production Dockerfile specifies `/usr/bin/tini` as `ENTRYPOINT`.
- `tini` assumes PID 1, handles sub-reaping automatically, and guarantees that any zombie children created by background sub-processes are immediately cleaned up.

---

## 2. Dynamic Linking & Libc Version Mismatch

### Context & Failure Mode
Compiling C++ or Rust against a newer glibc in the builder stage (e.g., Debian Trixie/Sid glibc 2.39) and attempting to run the binary inside an older runtime container (e.g., Debian Bookworm glibc 2.36) produces an immediate runtime crash:
```
/app/service-bin: /lib/x86_64-linux-gnu/libc.so.6: version `GLIBC_2.38' not found
```

### Mitigation Invariant
- **Symmetric Builder-Runtime OS Lineage**: Both the builder stage (`gcc:14-bookworm`, `rust:1.80-slim-bookworm`) and runtime stage (`debian:bookworm-slim`) are strictly anchored to the identical Debian Bookworm release.
- For Go, `CGO_ENABLED=0` guarantees a completely statically linked binary immune to glibc versioning.

---

## 3. Read-Only Root Filesystem Write Violations

### Context & Failure Mode
In accordance with Kubernetes security standards and `08-docker-file-writing-rule.md`, all containers run with `readOnlyRootFilesystem: true`. If an application or runtime engine attempts to write temporary files or logs to `/tmp`, `/var/log`, or the application directory `/app`, the OS denies the syscall with `EROFS: read-only file system`, crashing the service on startup.

### Mitigation Invariant
- All Dockerfiles declare explicit `tmpfs` mounts or mount non-persistent scratch areas under `/tmp`.
- Standard output (`stdout`) and standard error (`stderr`) are strictly used for all logging; no service creates local log files on the container root filesystem.

---

## 4. Signal Trapping Blindness & Hard OOM Kills

### Context & Failure Mode
If a Dockerfile defines an entrypoint using the shell format:
```dockerfile
ENTRYPOINT /app/service-bin
```
Docker wraps the command in `/bin/sh -c`. Shell processes do not forward `SIGTERM` or `SIGINT` signals to child processes by default. Consequently, when Kubernetes or Docker issues a graceful shutdown command, the service remains unaware until the timeout expires, resulting in an abrupt `SIGKILL` that terminates active database transactions and corrupts state.

### Mitigation Invariant
- All Dockerfiles strictly use the JSON array syntax:
```dockerfile
ENTRYPOINT ["/usr/bin/tini", "--", "/app/service-bin"]
```
This ensures direct signal routing to `tini`, which propagates the termination signal directly to the application loop.
