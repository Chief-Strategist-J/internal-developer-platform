# syntax=docker/dockerfile:1.7
ARG BASE_IMAGE=debian:bookworm-slim
ARG BUILDER_IMAGE=golang:1.27.3-bookworm

FROM ${BUILDER_IMAGE} AS builder
ARG SERVICE_PATH=.
WORKDIR /build

COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download || true

COPY src/ ./src/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /build/bin/platform-svc ./src/...

FROM ${BASE_IMAGE} AS runtime
ARG UID=10001
ARG GID=10001

WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tini \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd --gid ${GID} app 2>/dev/null || true \
 && useradd --uid ${UID} --gid ${GID} --no-create-home --shell /usr/sbin/nologin app 2>/dev/null || true \
 && chown -R ${UID}:${GID} /app

COPY --from=builder --chown=${UID}:${GID} /build/bin/platform-svc /app/platform-svc

USER ${UID}:${GID}
EXPOSE 8080
ENTRYPOINT ["/usr/bin/tini", "--", "/app/platform-svc"]
