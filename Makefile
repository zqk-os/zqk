# ZQK Community Makefile
# Overlay/export installs this file as dest/Makefile via
# scripts/open-core/install-community-makefile.sh.
# Binary basename comes from brand.executable_name (default zqk).
# TRACK: TDE-1789678536875854000-47240146

.PHONY: help all bootstrap-archive clean

.DEFAULT_GOAL := all

BRAND_CONFIG := $(firstword $(wildcard .zqk/config/config.yaml .zqk/config.yaml))
ifeq ($(BRAND_CONFIG),)
BRAND_EXE := zqk
else
BRAND_EXE := $(shell awk '/^[[:space:]]*executable_name:/{gsub(/["\047]/, "", $$2); print $$2; exit}' $(BRAND_CONFIG))
ifeq ($(strip $(BRAND_EXE)),)
BRAND_EXE := zqk
endif
endif

BIN = bin/$(BRAND_EXE)
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS = -X github.com/lanceman/zqk/cmd/zqk-community/app.version=$(VERSION) \
          -X github.com/lanceman/zqk/cmd/zqk-community/app.buildDate=$(BUILD_DATE) \
          -X github.com/lanceman/zqk/cmd/zqk-community/app.gitCommit=$(GIT_COMMIT)

help:
	@echo "ZQK Community — binary name is brand.executable_name ($(BRAND_EXE))"
	@echo ""
	@echo "  make / make all    Build ./$(BIN) from ./cmd/zqk-community"
	@echo "  make clean         Remove bin/*"
	@echo ""
	@echo "There is no make zqk-admin or promote-stable here."
	@echo "Scheduler CLI: ./$(BIN) scheduler start|stop|status"
	@echo "Run ./$(BIN) from this directory. Do not export ZQK_PROJECT_ROOT."

bootstrap-archive:
	@if [ -f scripts/build-bootstrap-archive.sh ]; then \
		$(SHELL) scripts/build-bootstrap-archive.sh "$$(pwd)" && echo "✓ Bootstrap archive ready"; \
	elif [ -f internal/bootstrap/archive/bootstrap.tar.gz ]; then \
		echo "✓ Pre-built bootstrap archive present"; \
	else \
		echo "Error: no bootstrap archive or builder found" >&2; exit 1; \
	fi

all: bootstrap-archive
	go build -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/zqk-community

clean:
	rm -rf bin/*
