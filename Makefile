# ZQK Makefile
# Overlay/export installs this file as dest/Makefile via
# scripts/open-core/install-community-makefile.sh.
# Binary basename comes from brand.executable_name in
# config/zqk-local.yaml (wins) then config/zqk.yaml (default zqk).

.PHONY: help all bootstrap-archive compile-bin zqk clean test test-unit test-unit-all test-race test-integration test-coverage test-coverage-html lint vet verify gate-release zqk-vet

.DEFAULT_GOAL := all

BRAND_CONFIG := $(firstword $(wildcard config/zqk-local.yaml config/zqk.yaml))
ifeq ($(BRAND_CONFIG),)
BRAND_EXE := zqk
else
BRAND_EXE := $(shell awk '/^[[:space:]]*executable_name:/{gsub(/["\047]/, "", $$2); print $$2; exit}' $(BRAND_CONFIG))
ifeq ($(strip $(BRAND_EXE)),)
BRAND_EXE := zqk
endif
endif

BIN = bin/$(BRAND_EXE)
BRAND_ENV_PREFIX := $(shell printf '%s' '$(BRAND_EXE)' | tr 'abcdefghijklmnopqrstuvwxyz-' 'ABCDEFGHIJKLMNOPQRSTUVWXYZ_')
ifeq ($(strip $(BRAND_ENV_PREFIX)),)
BRAND_ENV_PREFIX := ZQK
endif
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
# Public module path after the bounded community rewrite.
LDFLAGS = -X github.com/zqk-os/zqk/cmd/zqk/app.version=$(VERSION) \
          -X github.com/zqk-os/zqk/cmd/zqk/app.buildDate=$(BUILD_DATE) \
          -X github.com/zqk-os/zqk/cmd/zqk/app.gitCommit=$(GIT_COMMIT)

# Dynamic help generation
help:
	@echo "ZQK — binary name is brand.executable_name ($(BRAND_EXE))"
	@echo ""
	@echo "Build & Code Generation:"
	@echo "  make / make all       Build ./$(BIN)"
	@echo "  make codegen          Run spec & command builder codegen and refresh bootstrap archive"
	@echo "  make clean            Remove bin/*"
	@echo ""
	@echo "Verification & Gates (Single Source of Truth: config/gates.yaml):"
	@echo "  make lint             Run fast AST hygiene checks (paths, perms, CLI names)"
	@echo "  make vet              Run full verification engine (hygiene, tree_police, payload)"
	@echo "  make verify           Full pre-commit validation (vet + unit tests)"
	@echo "  make gate-release     Full release gate (verify + integration tests)"
	@echo ""
	@echo "Testing:"
	@echo "  make test             Run both unit and integration tests"
	@echo "  make test-unit        Run partitioned unit tests (pkg/... & cmd/...)"
	@echo "  make test-unit-all    Run full non-short unit tests"
	@echo "  make test-integration Run integration tests using built binary (release gates & CLI suites)"
	@echo "  make test-coverage    Run unit tests with coverage profile and print summary table"
	@echo "  make test-coverage-html Generate HTML visual test coverage report (coverage.html)"
	@echo ""
	@echo ""
	@echo "Scheduler CLI: ./$(BIN) scheduler start|stop|status"
	@echo "Run ./$(BIN) from this directory. Do not export a project-root environment variable."

bootstrap-archive:
	@if [ -f scripts/build-bootstrap-archive.sh ]; then \
		$(SHELL) scripts/build-bootstrap-archive.sh "$$(pwd)" && echo "✓ Bootstrap archive ready"; \
	elif [ -f internal/bootstrap/archive/bootstrap.tar.gz ]; then \
		echo "✓ Pre-built bootstrap archive present"; \
	else \
		echo "Error: no bootstrap archive or builder found" >&2; exit 1; \
	fi

compile-bin: bootstrap-archive
	go build -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/zqk
	ln -sf $(BRAND_EXE) bin/$(BRAND_EXE)-mcp-ide-adapter

codegen: compile-bin
	@echo "Generating builders from specs..."
	@$(BRAND_ENV_PREFIX)_DEV_CODEGEN=1 ./$(BIN) system generate-command-builders --overwrite --allow-degraded
	@$(BRAND_ENV_PREFIX)_DEV_CODEGEN=1 ./$(BIN) system generate-instance-builders --overwrite --allow-degraded
	@$(BRAND_ENV_PREFIX)_DEV_CODEGEN=1 ./$(BIN) system generate-config-builders --overwrite --allow-degraded
	@$(BRAND_ENV_PREFIX)_DEV_CODEGEN=1 ./$(BIN) system generate-api-builders --overwrite --allow-degraded
	@$(BRAND_ENV_PREFIX)_DEV_CODEGEN=1 ./$(BIN) system generate-profile-builders --overwrite --allow-degraded
	@$(BRAND_ENV_PREFIX)_DEV_CODEGEN=1 ./$(BIN) system generate-lifecycle-builders --overwrite --allow-degraded
	@$(BRAND_ENV_PREFIX)_DEV_CODEGEN=1 ./$(BIN) system generate-trait-builders --overwrite --allow-degraded
	@$(BRAND_ENV_PREFIX)_DEV_CODEGEN=1 ./$(BIN) system sync-id-prefixes-from-specs --apply --dry-run=false --allow-degraded
	-@$(BRAND_ENV_PREFIX)_DEV_CODEGEN=1 ./$(BIN) system sync-glossary-from-specs --apply --dry-run=false --allow-degraded
	@$(MAKE) bootstrap-archive
	@$(MAKE) compile-bin

all: codegen

build: all
build-all: all
zqk: compile-bin

test-unit:
	go test -short -p 2 -timeout 15m ./pkg/... ./cmd/... ./internal/... ./ext/...

test-unit-all:
	go test -p 2 -timeout 20m ./pkg/... ./cmd/... ./internal/... ./ext/...

test-race:
	go test -race -short -timeout 5m ./pkg/goroutinelabels/... ./pkg/concurrency/... ./pkg/bufferpool/...

test-integration: all
	$(BRAND_ENV_PREFIX)_SHARED_TEST_BIN="$$(pwd)/$(BIN)" sh scripts/open-core/test-public-release-gates.sh

test-coverage:
	go test -short -p 2 -timeout 15m -coverprofile=coverage.out -covermode=atomic ./pkg/... ./cmd/... ./internal/... ./ext/...
	python3 scripts/open-core/generate-coverage-summary.py coverage.out "Whole-Project Test Coverage"

test-coverage-html: test-coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage HTML report generated at coverage.html"
 
test: test-unit test-integration

bin/zqk-vet: $(shell find pkg/vet cmd/zqk-vet -name '*.go')
	@mkdir -p bin
	go build -o bin/zqk-vet ./cmd/zqk-vet

zqk-vet: bin/zqk-vet

lint: bin/zqk-vet
	./bin/zqk-vet --suite hygiene

vet: bin/zqk-vet
	./bin/zqk-vet --suite all

verify: vet test-unit

gate-release: vet test-integration

clean:
	rm -rf bin/* coverage.out coverage.html
