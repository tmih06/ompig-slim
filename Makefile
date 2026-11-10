GO ?= go
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: build build-broker run install check test race vet smoke smoke-built release benchmark test-install test-self-update

build:
	$(GO) build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim .

build-broker:
	$(GO) build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim-broker ./cmd/ompig-slim-broker

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
	python3 scripts/test-model-menu.py
	python3 scripts/smoke-test-model-menu.py ./bin/ompig-slim
	python3 scripts/measure-model-menu.py ./bin/ompig-slim --legacy-settings --runs 1
	python3 scripts/measure-model-menu.py ./bin/ompig-slim --stale-cycle --runs 1
	python3 scripts/smoke-test-codemode-binary.py ./bin/ompig-slim
	python3 scripts/smoke-test-subagents-binary.py ./bin/ompig-slim
	python3 scripts/smoke-test-web-tools-binary.py ./bin/ompig-slim

release:
	bash scripts/build-binaries.sh $(ARGS)

# Exercise the user-facing installer against a loopback release server; needs
# only Python 3, bash, and the usual archive/HTTP tools.
test-install:
	python3 scripts/test-install.py

# Prove `ompig-slim update` and `upgrade` on the real executable: they replace
# the file the installer placed, and refuse an installation it did not create.
# Builds no Go code and reaches no network beyond loopback.
test-self-update: build
	python3 scripts/test-self-update.py ./bin/ompig-slim

# Install the pinned baselines first; benchmark never rebuilds the measured CLI.
benchmark:
	python3 scripts/benchmark.py --slim ./bin/ompig-slim $(ARGS)
