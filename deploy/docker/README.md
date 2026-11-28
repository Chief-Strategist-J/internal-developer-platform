# Docker Deployment & Observability Guide

## Images & Dockerfiles (`deploy/docker/service/`)

- `Dockerfile`: `platform-api` (Go control-plane service, binary `/app/platform-svc`)
- `Dockerfile.typescript`: `developer-portal`
- `Dockerfile.rust`: `rust-worker`
- `Dockerfile.cpp`: `cpp-engine`

Published Docker Hub image:
- https://hub.docker.com/r/chiefj/platform-api (`chiefj/platform-api:latest`)

### Build & Push

```bash
# Build platform-api locally
DOCKER_BUILDKIT=1 docker build -f deploy/docker/service/Dockerfile -t chiefj/platform-api:latest .

# Or build all polyglot images
./scripts/build/build-all.sh

# Push platform-api to Docker Hub
docker push chiefj/platform-api:latest
```

## Docker Compose (`deploy/docker/compose/`)

- `docker-compose.infra.yml`: PostgreSQL, Redis, Tempo, Loki, Prometheus, OpenTelemetry Collector, Grafana, and Caddy proxy.
- `docker-compose.single.yml`: Single-service mode (`platform-api` + `migrate`) including `docker-compose.infra.yml`.
- `docker-compose.polyglot.yml`: Full polyglot stack (`platform-api`, `rust-worker`, `cpp-engine`, `developer-portal`) including `docker-compose.infra.yml`.

Start the stack using the Docker Hub image:

```bash
PLATFORM_API_IMAGE=chiefj/platform-api:latest \
  docker compose -f deploy/docker/compose/docker-compose.polyglot.yml up -d
```

> Note: Ensure local Kind port-forwards (ports `3000`, `3100`, `3200`, `4317`, `4318`, `5432`, `6379`, `8080`, `9090`) are stopped before starting Docker Compose to avoid port conflicts.

## Observability & Grafana

- **Prometheus scrape config**: `compose/prometheus.yaml` scrapes `prometheus` (`localhost:9090`), `otel-collector:8889`, and `platform-api` (`platform-api:9090`, `app:9090`).
- **Grafana datasources**: `compose/grafana-datasources.yaml` provisions Prometheus with `uid: Prometheus` (matching the dashboard definitions) along with Tempo and Loki.
- **Grafana UI**: `http://localhost:3000` (default credentials: `admin` / `admin`).
