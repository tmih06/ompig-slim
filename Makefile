GO ?= go
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: build run install check

build:
	$(GO) build -trimpath -o bin/pig .

run:
	$(GO) run . $(ARGS)

install:
	$(GO) build -trimpath -o "$(BINDIR)/pig" .

check:
	$(GO) test ./...
	$(GO) vet ./...
