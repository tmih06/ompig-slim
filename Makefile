GO ?= go
export GO
GOEXE := $(shell $(GO) env GOEXE)
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: deps build build-broker build-broker-image build-workspace-image run install check test race vet smoke smoke-built release release-publish benchmark test-install test-self-update test-broker-container test-broker-cluster test-broker-codex-usage test-extensions

build:
	$(GO) build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim$(GOEXE) .

# Unmodified dependencies are restored locally; the patched PiG tree stays tracked.
deps:
ifeq ($(OS),Windows_NT)
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/bootstrap-vendor.ps1
else
	bash scripts/bootstrap-vendor.sh
endif

build build-broker run install check test race vet: | deps

build-broker:
	$(GO) build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim-broker$(GOEXE) ./cmd/ompig-slim-broker

# The image is the deployable artifact for the storage service. Building it is
# separate from testing it so CI can test the exact image it publishes.
build-broker-image:
	docker build -f Dockerfile.broker -t ompig-slim-broker .

# The workspace image is the lightweight agent environment published with releases.
build-workspace-image:
	docker build -f Dockerfile.workspace -t ompig-slim-workspace .

run:
	$(GO) run -mod=vendor . $(ARGS)

install:
	$(GO) build -mod=vendor -trimpath -ldflags="-s -w" -o "$(BINDIR)/ompig-slim$(GOEXE)" .

check test race vet:
ifeq ($(OS),Windows_NT)
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/check-go.ps1 -Mode $@
else
	bash scripts/check-go.sh $@
endif

smoke: build
	$(MAKE) smoke-built

# CI extracts a verified release binary before calling this build-free target.
smoke-built:
ifeq ($(OS),Windows_NT)
	python scripts/smoke-test-windows.py ./bin/ompig-slim.exe
else
	python3 scripts/smoke-test-account-routing.py ./bin/ompig-slim
	python3 scripts/smoke-test-codex-account-policy.py ./bin/ompig-slim
	python3 scripts/smoke-test-model-discovery.py ./bin/ompig-slim
	python3 scripts/measure-startup.py ./bin/ompig-slim --isolated --model @default --runs 1
	python3 scripts/measure-resume.py ./bin/ompig-slim --turns 40 --runs 1
	python3 scripts/measure-resume.py ./bin/ompig-slim --turns 40 --via-picker --runs 1
	python3 scripts/smoke-test-compaction-fallback.py ./bin/ompig-slim
	python3 scripts/smoke-test-model-context.py ./bin/ompig-slim
	python3 scripts/smoke-test-durable.py ./bin/ompig-slim
	python3 scripts/test-model-menu.py
	python3 scripts/smoke-test-model-menu.py ./bin/ompig-slim
	python3 scripts/measure-model-switch.py ./bin/ompig-slim --runs 1 --rtt-ms 20 --check-ui
	python3 scripts/smoke-test-logout.py ./bin/ompig-slim
	python3 scripts/smoke-test-quit.py ./bin/ompig-slim
	python3 scripts/smoke-test-info-widgets.py ./bin/ompig-slim
	python3 scripts/smoke-test-mistral-usage.py ./bin/ompig-slim
	python3 scripts/smoke-test-mistral-content.py ./bin/ompig-slim
	python3 scripts/smoke-test-todo-arguments.py ./bin/ompig-slim
	python3 scripts/smoke-test-streaming-rules.py ./bin/ompig-slim
	python3 scripts/smoke-test-streaming-rules.py ./bin/ompig-slim --tui
	python3 scripts/measure-model-menu.py ./bin/ompig-slim --legacy-settings --runs 1
	python3 scripts/measure-model-menu.py ./bin/ompig-slim --stale-cycle --runs 1
	python3 scripts/smoke-test-opencode-free.py ./bin/ompig-slim
	python3 scripts/smoke-test-codemode-binary.py ./bin/ompig-slim
	python3 scripts/smoke-test-gemini-decisions.py ./bin/ompig-slim
	python3 scripts/smoke-test-subagents-binary.py ./bin/ompig-slim
	python3 scripts/smoke-test-subagent-ui.py ./bin/ompig-slim
	python3 scripts/smoke-test-web-tools-binary.py ./bin/ompig-slim
	python3 scripts/smoke-test-agent-state.py ./bin/ompig-slim
	python3 scripts/smoke-test-goal-interruption.py ./bin/ompig-slim
	python3 scripts/smoke-test-goal-interruption.py ./bin/ompig-slim --quota
	python3 scripts/smoke-test-goal-blockers.py ./bin/ompig-slim
	python3 scripts/smoke-test-goal-ui.py ./bin/ompig-slim
endif

release:
	bash scripts/build-binaries.sh $(ARGS)

# Prepare a versioned commit and publish it only when VERSION is explicit.
release-publish:
	bash scripts/release.sh $(VERSION) --publish

# Exercise the user-facing installer against a loopback release server; needs
# only Python 3, bash, and the usual archive/HTTP tools. The Windows suite also
# drives the installed executable, so it builds one first.
ifeq ($(OS),Windows_NT)
test-install: build
endif
test-install:
ifeq ($(OS),Windows_NT)
	python scripts/test-install-windows.py ./bin/ompig-slim.exe
else
	python3 scripts/test-install.py
endif

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
ifeq ($(OS),Windows_NT)
	python scripts/test-install-windows.py ./bin/ompig-slim.exe
else
	python3 scripts/test-self-update.py ./bin/ompig-slim
endif

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

test-broker-codex-usage: build build-broker
	python3 scripts/smoke-test-broker-codex-usage.py ./bin/ompig-slim ./bin/ompig-slim-broker

# Install the pinned baselines first; benchmark never rebuilds the measured CLI.
benchmark:
	python3 scripts/benchmark.py --slim ./bin/ompig-slim $(ARGS)
