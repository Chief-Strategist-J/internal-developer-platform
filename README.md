# internal-developer-platform
Kubernetes, GitOps, Terraform, ArgoCD

## Container Image

- **Docker Hub**: https://hub.docker.com/r/chiefj/platform-api (`chiefj/platform-api:latest`)

```bash
docker pull chiefj/platform-api:latest
```

To run Docker Compose with the Docker Hub image instead of the default `ghcr.io/scaibu/platform-api:latest`:

```bash
PLATFORM_API_IMAGE=chiefj/platform-api:latest docker compose -f deploy/docker/compose/docker-compose.polyglot.yml up -d
```

## Observability & Grafana Dashboards

- **Metrics endpoint**: `platform-api` exposes Prometheus metrics on `:9090/metrics` (and `:8081/metrics`).
- **Grafana** (Kind `dev` namespace):
  - Forward local ports with `./scripts/run/port-forward.sh` (or `kubectl -n dev port-forward svc/grafana 3000:3000`).
  - Open `http://localhost:3000` (default credentials: `admin` / `admin`) and select the **Platform** folder (`Self-Service Provisioning — SLIs, SLOs & Saga Operations`).
