.PHONY: bench lint help build-all zqk zqk-admin zqk-community zcom zqk-mcp generate-spec-builders clean scheduler-stop scheduler-restart install-scheduler-service release-snapshot promote-stable test-cases

ZQK_BIN = bin/zqk
ZCOM_BIN = bin/zcom
ZQK_ADMIN_BIN = bin/zqk-admin
ZQK_MCP_BIN = bin/zqk-mcp
ZQK_MCP_FAL_BIN = bin/zqk-mcp-fal
ZQK_SHIM_BIN = bin/zqk-shim

# Version injection: captured at build time, embedded via ldflags.
# GoReleaser uses its own template variables for release builds; these are for local dev.
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS     = -X github.com/lanceman/zqk/cmd/zqk/app.version=$(VERSION) \
              -X github.com/lanceman/zqk/cmd/zqk/app.buildDate=$(BUILD_DATE) \
              -X github.com/lanceman/zqk/cmd/zqk/app.gitCommit=$(GIT_COMMIT)
COMMUNITY_LDFLAGS = -X github.com/lanceman/zqk/cmd/zqk-community/app.version=$(VERSION) \
              -X github.com/lanceman/zqk/cmd/zqk-community/app.buildDate=$(BUILD_DATE) \
              -X github.com/lanceman/zqk/cmd/zqk-community/app.gitCommit=$(GIT_COMMIT)

# Seated community product has cmd/zqk-community and no studio main.
ifneq ($(wildcard cmd/zqk-community/main.go),)
ifeq ($(wildcard cmd/zqk/main.go),)
.DEFAULT_GOAL := zcom
endif
endif

# POL-AGENT-ACCOUNT-LOGIN-001: AuthMiddleware rejects legacy account:* keys.
# Prefer ACC-* / issued secrets from the environment or .env; do not fall back to hardcoded system account.
ADMIN_AUTH = set -a; [ -f .env ] && . ./.env; set +a; \
	export ZQK_ALLOW_UNBOUND_ACCOUNT=$${ZQK_ALLOW_UNBOUND_ACCOUNT:-1}; \
	_k=""; \
	for _c in "$${ZQK_ADMIN_API_KEY-}" "$${ZQK_API_KEY-}" "$${ZQK_SYSTEM_ACC-}"; do \
	  case "$$_c" in \
	    "") continue ;; \
	    account:*) continue ;; \
	    ACC-*|zqk_ak_*) _k="$$_c"; break ;; \
	  esac; \
	done; \
	if [ -n "$$_k" ]; then \
	  export ZQK_ADMIN_API_KEY="$$_k"; export ZQK_API_KEY=$${ZQK_API_KEY:-$$_k}; \
	fi;

# Default target
help:
	@if [ ! -f cmd/zqk/main.go ]; then \
		echo "ZQK Community pressure-test (zcom) — not studio zqk"; \
		echo ""; \
		echo "  make / make zcom            - Build bin/zcom from ./cmd/zqk-community"; \
		echo "  make zqk-community          - Same SKU as zcom plus portable bootstrap verify"; \
		echo "  make clean                  - Remove bin/*"; \
		echo ""; \
		echo "Use ./bin/zcom with ZCOM_PROJECT_ROOT pointing at this checkout."; \
		echo "Studio remains ./bin/zqk. Rename zcom → zqk only at public launch."; \
	else \
		echo "ZQK Build System"; \
		echo ""; \
		echo "  make build-all              - Full build (binaries + generate-spec-builders + promote-stable)"; \
		echo "  make zqk                    - Build bin/zqk (embeds bootstrap archive)"; \
		echo "  make zqk-community          - Build bin/zqk-community + portable bootstrap verify"; \
		echo "  make zcom                    - Community SKU: bin/zcom from ./cmd/zqk-community"; \
		echo "  make zqk-admin              - Build bin/zqk-admin"; \
		echo "  make zqk-mcp / zqk-mcp-fal  - Build MCP binaries"; \
		echo "  make zqk-neuron|muscle|heart|lung - Tier-specialized zqk binaries"; \
		echo "  make zqk-shim               - Build bin/zqk-shim (+ bin/shims links)"; \
		echo "  make generate-spec-builders - Regenerate builders via zqk-admin"; \
		echo "  make promote-stable         - Stop holders, install tip → zqk-stable, recycle ALL stable daemons"; \
		echo "  make verify                 - spec-driven test_case verification + golangci-lint + govulncheck"; \
		echo "  make test-cases             - Run all active kernel test_case objects"; \
		echo "  make govulncheck            - Run govulncheck on all packages"; \
		echo "  make gosec                  - Run gosec security scanner"; \
		echo "  make clean                  - Remove bin/* and .zqk/tmp/*"; \
		echo "  make scheduler-stop|restart - Stop/start local scheduler daemon"; \
		echo "  make install-scheduler-service - Host OS unit (launchd/systemd)"; \
		echo "  make release VERSION=vX.Y.Z - Release packaging (see scripts/release.sh)"; \
		echo "  make release-community VERSION=vX.Y.Z - Community package"; \
		echo ""; \
	fi

build-all: zqk zqk-admin zqk-community zqk-mcp zqk-mcp-fal generate-spec-builders zqk-neuron zqk-muscle zqk-heart zqk-lung zqk-shim promote-stable

# Stop scheduler before rebuilding bin/zqk to prevent destabilizing the running daemon.
# Uses -f so it returns 0 even if not running. Restarts after build via scheduler-restart.
scheduler-stop:
	@-$(ADMIN_AUTH) ./bin/zqk scheduler stop --force --quiet 2>/dev/null; true

scheduler-restart:
	@-$(ADMIN_AUTH) ./bin/zqk scheduler start --quiet 2>/dev/null; true

bootstrap-archive:
	@$(SHELL) scripts/build-bootstrap-archive.sh "$$(pwd)" && echo "✓ Bootstrap archive ready"

zqk: bootstrap-archive
	rm -f $(ZQK_BIN)
	go build -ldflags '$(LDFLAGS)' -o $(ZQK_BIN) ./cmd/zqk

zqk-neuron:
	GOFLAGS="-ldflags=-X=github.com/lanceman/zqk/pkg/specialization.DefaultTier=neuron" go build -o bin/zqk-neuron ./cmd/zqk

zqk-muscle:
	GOFLAGS="-ldflags=-X=github.com/lanceman/zqk/pkg/specialization.DefaultTier=muscle" go build -o bin/zqk-muscle ./cmd/zqk

zqk-heart:
	GOFLAGS="-ldflags=-X=github.com/lanceman/zqk/pkg/specialization.DefaultTier=heart" go build -o bin/zqk-heart ./cmd/zqk

zqk-lung:
	GOFLAGS="-ldflags=-X=github.com/lanceman/zqk/pkg/specialization.DefaultTier=lung" go build -o bin/zqk-lung ./cmd/zqk

# Community binary embeds scrubbed bootstrap (REQ-9009). Never treat this monorepo
# as the user's project root — verify extract only into a temp project.
zqk-community: bootstrap-archive
	go build -ldflags '$(COMMUNITY_LDFLAGS)' -o bin/zqk-community ./cmd/zqk-community
	@$(SHELL) scripts/open-core/verify-bootstrap-portable.sh "$$(pwd)" bin/zqk-community

# Community binary embeds scrubbed bootstrap (REQ-9009). Never treat this monorepo
# as the user's project root — verify extract only into a temp project.
# Pressure-test name is zcom so it cannot be confused with studio zqk.
zcom: bootstrap-archive
	go build -ldflags '$(COMMUNITY_LDFLAGS)' -o $(ZCOM_BIN) ./cmd/zqk-community

zqk-admin:
	go build -ldflags '$(LDFLAGS)' -o $(ZQK_ADMIN_BIN) ./cmd/zqk-admin

zqk-mcp:
	go build -ldflags '$(LDFLAGS)' -o $(ZQK_MCP_BIN) ./cmd/zqk/mcp-simple

zqk-mcp-fal:
	go build -o $(ZQK_MCP_FAL_BIN) ./cmd/utilities/zqk-mcp-fal

zqk-shim:
	go build -o $(ZQK_SHIM_BIN) ./cmd/zqk-shim
	mkdir -p bin/shims
	ln -sf ../zqk-shim bin/shims/git
	ln -sf ../zqk-shim bin/shims/gh

generate-spec-builders: zqk-admin
	@echo "Generating specification builders from schema documents..."
	@$(ADMIN_AUTH) ./bin/zqk-admin system generate-command-builders --overwrite --allow-degraded
	@$(ADMIN_AUTH) ./bin/zqk-admin system generate-instance-builders --overwrite --allow-degraded
	@$(ADMIN_AUTH) ./bin/zqk-admin system generate-config-builders --overwrite --allow-degraded
	@$(ADMIN_AUTH) ./bin/zqk-admin system generate-api-builders --overwrite --allow-degraded
	@$(ADMIN_AUTH) ./bin/zqk-admin system generate-profile-builders --overwrite --allow-degraded
	@$(ADMIN_AUTH) ./bin/zqk-admin system generate-lifecycle-builders --overwrite --allow-degraded
	@$(ADMIN_AUTH) ./bin/zqk-admin system sync-id-prefixes-from-specs --apply --dry-run=false --allow-degraded
	-@$(ADMIN_AUTH) ./bin/zqk-admin system sync-glossary-from-specs --apply --dry-run=false --allow-degraded
	@echo "Synchronizing CLI Help golden text baselines..."
	ZQKCLI_TEST_UPDATE_HELP_GOLDEN=1 ZQK_ADMIN_TEST_INIT_API_KEY=mock_token go test ./pkg/zqkcli/... -run TestHelpMenuParity/GoldenFileComparison || true

sync-glossary: zqk-admin
	@$(ADMIN_AUTH) ./$(ZQK_ADMIN_BIN) system sync-glossary-from-specs --apply --dry-run=false --allow-degraded

clean:
	rm -rf bin/* .zqk/tmp/*

# Promote tip → stable. Always stop holders first: MCP runs as zqk-mcp-daemon (symlink to
# zqk-stable) and otherwise blocks install-zqk-stable.sh. Then recycle EVERY long-lived
# stable consumer (scheduler, MCP, PrivilegedWriter, seat-workers) — mcp ensure alone is
# idempotent and leaves stale inodes. TRACK: TDE-1785808957221945000-fcd15e47
promote-stable: zqk
	./scripts/install-zqk-stable.sh --stop-scheduler --from $(ZQK_BIN)
	@./scripts/recycle-stable-daemons.sh

# Local CI: checkout committed SHA into .zqk/local-ci/workdir (does not schedule tests)
local-ci-checkout:
	./scripts/local-ci-checkout.sh $(LOCAL_CI_ARGS)

# Alias used in docs / habit (same as local-ci-checkout)
promote-test-source: local-ci-checkout

# Software Bill of Materials (SBOM) generation (SPDX 2.3 JSON)
sbom:
	./scripts/generate-sbom.sh

# Generate cryptographic sha256 checksums for built binaries
checksums:
	./scripts/verify-binary-checksums.sh --generate

# Verify binary checksums against manifest
verify-checksums:
	./scripts/verify-binary-checksums.sh

verify-tdd:
	go run ./cmd/utilities/drift_sensor_tdd

verify-scenarios: zqk
	@echo "Running programmatic policy enforcement scenarios..."
	@echo "Skipping missing scenario: scripts/scenarios/policy_enforcement/observability_check.yaml"
	# ./bin/zqk system validate-scenario --file scripts/scenarios/policy_enforcement/observability_check.yaml

install-vulncheck:
	go install golang.org/x/vuln/cmd/govulncheck@latest

install-gosec:
	go install github.com/securego/gosec/v2/cmd/gosec@latest

govulncheck: install-vulncheck
	$$(go env GOPATH)/bin/govulncheck ./...

# Run spec-driven test_case verification suite across the kernel
test-cases: zqk
	./bin/zqk test run --all

verify-supply-gates:
	@echo "Verifying deterministic SBOM generation and binary checksum fail-closed gates..."
	./scripts/test-supply-sbom-generation.sh
	./scripts/test-supply-checksum-verification.sh

# Modern verify: TDD drift sensor + scenarios + spec-driven test cases + vulnerability + lint + supply gates
verify: zqk verify-tdd verify-scenarios verify-supply-gates install-vulncheck lint test-cases
	$$(go env GOPATH)/bin/govulncheck ./... || { ./scripts/cleanup_orphan_test_processes.sh --kill || true; exit 1; }
	golangci-lint run ./... || { ./scripts/cleanup_orphan_test_processes.sh --kill || true; exit 1; }
	./scripts/cleanup_orphan_test_processes.sh --kill || true

# Run legacy scheduler scan-tests bundle matrix (deprecated under REQ-TEST-BUNDLE-DEPRECATION-001; use 'make verify')
verify-legacy-bundles: zqk
	./bin/zqk scheduler scan-tests --all --save-bundle make-verify --overwrite || { ./scripts/cleanup_orphan_test_processes.sh --kill || true; exit 1; }
	./scripts/test_bundle_matrix_pipeline.py --bundle-prefix make-verify --wait-health 1800 --strict --convergence-fail || { ./scripts/cleanup_orphan_test_processes.sh --kill || true; exit 1; }

# Install per-root host OS unit (launchd/systemd). Replaces retired ensure-cron.
# See docs/architecture/SCHEDULER_HOST_SERVICE_AND_CLUSTER_STATUS.md
install-scheduler-service:
	@$(ADMIN_AUTH) ./bin/zqk scheduler service install --root "$$(pwd)"


install-watchdog-cron:
	@REPO_ROOT=$$(pwd); \
	GO_BIN=$$(which go || echo "go"); \
	CRON_LINE="* * * * * cd $$REPO_ROOT && $$GO_BIN run ./cmd/utilities/run_watchdog_monitor >> .zqk/logs/watchdog-cron.log 2>&1"; \
	(crontab -l 2>/dev/null | grep -v run_watchdog_monitor; echo "$$CRON_LINE") | crontab -; \
	echo "Installed out-of-band watchdog cron (every 1 min). View: crontab -l"

# Build release artifacts locally and create a GitHub release.
# Usage: make release VERSION=v2.7.0
# Usage: make release-dry VERSION=v2.7.0   (build only, no GitHub push)
release:
	@test -n "$(VERSION)" || (echo "Usage: make release VERSION=v2.7.0" && exit 1)
	./scripts/release.sh $(VERSION)

release-dry:
	@test -n "$(VERSION)" || (echo "Usage: make release-dry VERSION=v2.7.0" && exit 1)
	./scripts/release.sh $(VERSION) --dry

release-community:
	@test -n "$(VERSION)" || (echo "Usage: make release-community VERSION=v2.7.0" && exit 1)
	./scripts/package-community.sh $(VERSION)

release-community-dry:
	@test -n "$(VERSION)" || (echo "Usage: make release-community-dry VERSION=v2.7.0" && exit 1)
	./scripts/package-community.sh $(VERSION) --dry

# Build release artifacts with goreleaser (optional, if installed).
release-snapshot:
	goreleaser release --snapshot --clean

configure-restored:
	./scripts/configure-restored-project.sh

configure-restored-check:
	./scripts/configure-restored-project.sh --check


lint:
	golangci-lint run ./...

bench:
	go test -bench=. ./...
