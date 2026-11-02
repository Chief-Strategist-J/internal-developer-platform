# syntax=docker/dockerfile:1.7
ARG BASE_IMAGE=debian:bookworm-slim
ARG BUILDER_IMAGE=gcc:16.2.0-bookworm

FROM ${BUILDER_IMAGE} AS builder
RUN apt-get update && apt-get install -y --no-install-recommends cmake make \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /build
COPY cpp-modules/ ./
RUN cmake -B build -DCMAKE_BUILD_TYPE=Release \
 && cmake --build build

FROM ${BASE_IMAGE} AS runtime
ARG UID=10001
ARG GID=10001

WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tini \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd --gid ${GID} app 2>/dev/null || true \
 && useradd --uid ${UID} --gid ${GID} --no-create-home --shell /usr/sbin/nologin app 2>/dev/null || true \
 && chown -R ${UID}:${GID} /app

COPY --from=builder --chown=${UID}:${GID} /build/build/*-engine /app/service-bin

USER ${UID}:${GID}
EXPOSE 9090
ENTRYPOINT ["/usr/bin/tini", "--", "/app/service-bin"]
