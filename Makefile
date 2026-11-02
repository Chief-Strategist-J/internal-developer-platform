.PHONY: all build test clean docker-build help

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

clean:
	@echo "==> Cleaning build artifacts..."
	rm -rf dist/ cpp-modules/build/ typescript-modules/dist/ typescript-modules/node_modules/
