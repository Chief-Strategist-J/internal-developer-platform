.PHONY: all build test clean docker-build help k8s-render k8s-cluster k8s-load k8s-up k8s-status k8s-down k8s-destroy

all: build test

build: build-go build-ts build-rust build-cpp

build-go:
	@echo "==> Compiling Go platform core..."
	go build -v ./...

build-ts:
	@echo "==> Compiling TypeScript modules..."
	cd typescript-modules && npm run build --if-present

build-rust:
	@echo "==> Compiling Rust modules..."
	if command -v cargo >/dev/null 2>&1; then cd rust-modules && cargo build --release; else echo "cargo not found on host, build via Docker"; fi

build-cpp:
	@echo "==> Compiling C++ modules..."
	if command -v cmake >/dev/null 2>&1; then cd cpp-modules && cmake -B build && cmake --build build; else echo "cmake not found on host, build via Docker"; fi

test:
	@echo "==> Executing Go unit tests..."
	go test -v ./...

docker-build:
	@echo "==> Building Docker images via build script..."
	./scripts/build/build-all.sh

docker-build-all: docker-build

K8S_OVERLAY ?= deploy/k8s/overlays/local-polyglot
K8S_NAMESPACE ?= idp
KIND_CLUSTER ?= idp
IDP_IMAGES ?= ghcr.io/scaibu/platform-api:latest ghcr.io/scaibu/rust-worker:latest ghcr.io/scaibu/cpp-engine:latest ghcr.io/scaibu/developer-portal:latest

k8s-render:
	@echo "==> Rendering $(K8S_OVERLAY)..."
	kubectl kustomize $(K8S_OVERLAY)

k8s-cluster:
	@echo "==> Creating kind cluster $(KIND_CLUSTER)..."
	kind get clusters | grep -qx $(KIND_CLUSTER) || kind create cluster --config deploy/k8s/local/kind-config.yaml

k8s-load:
	@echo "==> Loading service images into kind cluster $(KIND_CLUSTER)..."
	for image in $(IDP_IMAGES); do kind load docker-image $$image --name $(KIND_CLUSTER); done

k8s-up: k8s-cluster
	@echo "==> Deploying $(K8S_OVERLAY) to namespace $(K8S_NAMESPACE)..."
	kubectl delete job platform-migrate -n $(K8S_NAMESPACE) --ignore-not-found
	kubectl apply -k $(K8S_OVERLAY)
	kubectl rollout status statefulset/db -n $(K8S_NAMESPACE) --timeout=180s
	kubectl wait --for=condition=complete job/platform-migrate -n $(K8S_NAMESPACE) --timeout=300s
	kubectl rollout status deployment --timeout=180s -n $(K8S_NAMESPACE)

k8s-status:
	kubectl get pods,svc,job -n $(K8S_NAMESPACE)

k8s-down:
	@echo "==> Removing $(K8S_OVERLAY)..."
	kubectl delete -k $(K8S_OVERLAY) --ignore-not-found

k8s-destroy:
	@echo "==> Deleting kind cluster $(KIND_CLUSTER)..."
	kind delete cluster --name $(KIND_CLUSTER)

clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf dist/ cpp-modules/build/ typescript-modules/dist/ typescript-modules/node_modules/
