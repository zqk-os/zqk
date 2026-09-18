# ZQK Community pressure-test (zcom) — not studio zqk.
# Overlay/export MUST install this file as dest/Makefile via
# scripts/open-core/install-community-makefile.sh.
# Do not copy studio Makefile into the community tree.
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
	@echo "  make / make zcom   Build ./bin/zcom from ./cmd/zqk-community"
	@echo "  make clean         Remove bin/*"
	@echo ""
	@echo "There is no make zqk, zqk-admin, zqk-mcp, or promote-stable here."
	@echo "Scheduler CLI: ./bin/zcom scheduler start|stop|status"
	@echo "Run ./bin/zcom from this directory. Do not export ZCOM_PROJECT_ROOT."
	@echo "Rename zcom → zqk only at public launch."

bootstrap-archive:
	@if [ -f scripts/build-bootstrap-archive.sh ]; then \
		$(SHELL) scripts/build-bootstrap-archive.sh "$$(pwd)" && echo "✓ Bootstrap archive ready"; \
	elif [ -f internal/bootstrap/archive/bootstrap.tar.gz ]; then \
		echo "✓ Pre-built bootstrap archive present"; \
	else \
		echo "Error: no bootstrap archive or builder found" >&2; exit 1; \
	fi

zcom: bootstrap-archive
	go build -ldflags '$(LDFLAGS)' -o $(ZCOM_BIN) ./cmd/zqk-community

clean:
	rm -rf bin/*
