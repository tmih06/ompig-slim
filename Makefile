GO ?= go
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: build run install check test race vet smoke release

build:
	$(GO) build -mod=vendor -trimpath -ldflags="-s -w" -o bin/ompig-slim .

run:
	$(GO) run -mod=vendor . $(ARGS)

install:
	$(GO) build -mod=vendor -trimpath -ldflags="-s -w" -o "$(BINDIR)/ompig-slim" .

check test race vet:
	GO="$(GO)" bash scripts/check-go.sh $@

smoke: build
	python3 scripts/smoke-test-codemode-binary.py ./bin/ompig-slim
	python3 scripts/smoke-test-subagents-binary.py ./bin/ompig-slim

release:
	bash scripts/build-binaries.sh $(ARGS)
