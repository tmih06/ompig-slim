GO ?= go
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: build run install check test race vet smoke smoke-built release benchmark

build:
	$(GO) build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim .

run:
	$(GO) run -mod=vendor . $(ARGS)

install:
	$(GO) build -mod=vendor -trimpath -ldflags="-s -w" -o "$(BINDIR)/ompig-slim" .

check test race vet:
	GO="$(GO)" bash scripts/check-go.sh $@

smoke: build
	$(MAKE) smoke-built

# CI extracts a verified release binary before calling this build-free target.
smoke-built:
	python3 scripts/smoke-test-codemode-binary.py ./bin/ompig-slim
	python3 scripts/smoke-test-subagents-binary.py ./bin/ompig-slim
	python3 scripts/smoke-test-web-tools-binary.py ./bin/ompig-slim

release:
	bash scripts/build-binaries.sh $(ARGS)

# Install the pinned baselines first; benchmark never rebuilds the measured CLI.
benchmark:
	python3 scripts/benchmark.py --slim ./bin/ompig-slim $(ARGS)
