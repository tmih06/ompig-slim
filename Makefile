GO ?= go
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: build build-broker build-broker-image run install check test race vet smoke smoke-built release benchmark test-install test-self-update test-broker-container test-broker-cluster test-extensions

build:
	$(GO) build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim .

build-broker:
	$(GO) build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim-broker ./cmd/ompig-slim-broker

# The image is the deployable artifact for the storage service. Building it is
# separate from testing it so CI can test the exact image it publishes.
build-broker-image:
	docker build -f Dockerfile.broker -t ompig-slim-broker .

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
	python3 scripts/smoke-test-info-widgets.py ./bin/ompig-slim
	python3 scripts/smoke-test-todo-arguments.py ./bin/ompig-slim
	python3 scripts/measure-model-menu.py ./bin/ompig-slim --legacy-settings --runs 1
	python3 scripts/measure-model-menu.py ./bin/ompig-slim --stale-cycle --runs 1
	python3 scripts/smoke-test-codemode-binary.py ./bin/ompig-slim
	python3 scripts/smoke-test-subagents-binary.py ./bin/ompig-slim
	python3 scripts/smoke-test-web-tools-binary.py ./bin/ompig-slim
	python3 scripts/smoke-test-agent-state.py ./bin/ompig-slim

release:
	bash scripts/build-binaries.sh $(ARGS)

# Exercise the user-facing installer against a loopback release server; needs
# only Python 3, bash, and the usual archive/HTTP tools.
test-install:
	python3 scripts/test-install.py

# Run the installable extensions' own test suites. They live in a separate Go
# module per extension because PiG builds each one against the SDK staged for the
# running version, which a test needs a `replace` for; the script adds and
# removes it. The adapters themselves ship as extensions, not in the binary.
test-extensions: build
	bash scripts/test-extensions.sh

# Prove `ompig-slim update` and `upgrade` on the real executable: they replace
# the file the installer placed, and refuse an installation it did not create.
# Builds no Go code and reaches no network beyond loopback.
test-self-update: build
	python3 scripts/test-self-update.py ./bin/ompig-slim

# Drive the deployable container: authorization, revisions, the credential lease,
# restart durability and two real clients. Needs a Docker daemon; builds the
# image and the client executable first, and touches no user profile.
test-broker-container: build
	python3 scripts/test-broker-container.py --binary ./bin/ompig-slim

# The deployed shape: one broker host and two client containers on a private
# network. Needs a Docker daemon; builds the broker image and a client image from
# this checkout, then drives synchronization, credentials and connection setup.
test-broker-cluster: build
	python3 scripts/test-broker-cluster.py --binary ./bin/ompig-slim

# Install the pinned baselines first; benchmark never rebuilds the measured CLI.
benchmark:
	python3 scripts/benchmark.py --slim ./bin/ompig-slim $(ARGS)
