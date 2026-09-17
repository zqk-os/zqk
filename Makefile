# ZQK Community pressure-test (zcom) — not studio zqk.
# Overlay/export install this file as the product Makefile via
# scripts/open-core/install-community-makefile.sh.
# TRACK: TDE-1789678536875854000-47240146 — rename zcom → zqk at public launch.

.PHONY: help zcom bootstrap-archive clean

.DEFAULT_GOAL := zcom

ZCOM_BIN = bin/zcom
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS = -X github.com/lanceman/zqk/cmd/zqk-community/app.version=$(VERSION) \
          -X github.com/lanceman/zqk/cmd/zqk-community/app.buildDate=$(BUILD_DATE) \
          -X github.com/lanceman/zqk/cmd/zqk-community/app.gitCommit=$(GIT_COMMIT)

help:
	@echo "ZQK Community pressure-test (zcom) — not studio zqk"
	@echo ""
	@echo "  make / make zcom   - Build bin/zcom from ./cmd/zqk-community"
	@echo "  make clean          - Remove bin/*"
	@echo ""
	@echo "Use ./bin/zcom from the project directory."
	@echo "Do not export ZCOM_PROJECT_ROOT in your shell profile — it hijacks cwd."
	@echo "Studio remains ./bin/zqk. Rename zcom → zqk only at public launch."

bootstrap-archive:
	@$(SHELL) scripts/build-bootstrap-archive.sh "$$(pwd)" && echo "✓ Bootstrap archive ready"

zcom: bootstrap-archive
	go build -ldflags '$(LDFLAGS)' -o $(ZCOM_BIN) ./cmd/zqk-community

clean:
	rm -rf bin/*
