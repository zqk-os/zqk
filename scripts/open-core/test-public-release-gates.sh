#!/bin/sh
set -eu

MODE=full
if [ "${1:-}" = "--payload-only" ]; then
	MODE=payload-only
	shift
fi
ROOT=${1:-}
if [ -z "$ROOT" ]; then
	ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)
TMP_BIN=$(mktemp "${TMPDIR:-/tmp}/zqk-public-gate.XXXXXX")
TMP_REPO=$(mktemp -d "${TMPDIR:-/tmp}/zqk-public-gate-repo.XXXXXX")
trap 'rm -f "$TMP_BIN"; rm -rf "$TMP_REPO"' EXIT HUP INT TERM

sh -n "$ROOT/scripts/open-core/check-public-release-payload.sh"
sh "$ROOT/scripts/open-core/check-public-release-payload.sh" "$ROOT"

# Prove the gate fails closed on an explicitly forbidden release file.
mkdir -p "$TMP_REPO/cmd/zqk" "$TMP_REPO/config" "$TMP_REPO/docs/onboarding" "$TMP_REPO/scripts/open-core"
for path in README.md LICENSE NOTICE SECURITY.md CODE_OF_CONDUCT.md CONTRIBUTING.md; do
	printf '%s\n' "fixture" >"$TMP_REPO/$path"
done
printf '%s\n' 'module github.com/zqk-os/zqk' >"$TMP_REPO/go.mod"
printf '%s\n' 'brand:' '  executable_name: zqk' >"$TMP_REPO/config/zqk.yaml"
printf '%s\n' 'package main' >"$TMP_REPO/cmd/zqk/main.go"
printf '%s\n' 'fixture' >"$TMP_REPO/docs/INDEX.md"
printf '%s\n' 'fixture' >"$TMP_REPO/docs/onboarding/COMMUNITY_FIRST_RUN.md"
cp "$ROOT/scripts/open-core/check-public-release-payload.sh" "$TMP_REPO/scripts/open-core/"
cat >"$TMP_REPO/scripts/open-core/police-community-tree.sh" <<'EOF'
#!/bin/sh
exit 0
EOF
: >"$TMP_REPO/cmd/zqk-admin"
git -C "$TMP_REPO" init -q
git -C "$TMP_REPO" add .
if sh "$TMP_REPO/scripts/open-core/check-public-release-payload.sh" "$TMP_REPO" >"$TMP_REPO/result.log" 2>&1; then
	printf '%s\n' "payload gate accepted forbidden cmd/zqk-admin" >&2
	exit 1
fi
grep -F 'forbidden or unconfigured public artifact present: cmd/zqk-admin' "$TMP_REPO/result.log" >/dev/null

# Prove TRACK comments with kernel object ids fail closed (cmd/, not only scripts/).
TRACK_REPO=$(mktemp -d "${TMPDIR:-/tmp}/zqk-public-gate-track.XXXXXX")
trap 'rm -f "$TMP_BIN"; rm -rf "$TMP_REPO" "$TRACK_REPO"' EXIT HUP INT TERM
mkdir -p "$TRACK_REPO/cmd/zqk" "$TRACK_REPO/cmd/zqk-shim" "$TRACK_REPO/config" \
	"$TRACK_REPO/docs/onboarding" "$TRACK_REPO/scripts/open-core"
for path in README.md LICENSE NOTICE SECURITY.md CODE_OF_CONDUCT.md CONTRIBUTING.md; do
	printf '%s\n' "fixture" >"$TRACK_REPO/$path"
done
printf '%s\n' 'module github.com/zqk-os/zqk' >"$TRACK_REPO/go.mod"
printf '%s\n' 'brand:' '  executable_name: zqk' >"$TRACK_REPO/config/zqk.yaml"
printf '%s\n' 'package main' >"$TRACK_REPO/cmd/zqk/main.go"
printf '%s\n' 'fixture' >"$TRACK_REPO/docs/INDEX.md"
printf '%s\n' 'fixture' >"$TRACK_REPO/docs/onboarding/COMMUNITY_FIRST_RUN.md"
cp "$ROOT/scripts/open-core/check-public-release-payload.sh" "$TRACK_REPO/scripts/open-core/"
cat >"$TRACK_REPO/scripts/open-core/police-community-tree.sh" <<'EOF'
#!/bin/sh
exit 0
EOF
# Assemble the leak without embedding a TRACK: BLI-... literal in this gate script
# (the payload scan also covers scripts/).
{
	printf '%s\n' 'package main'
	printf '%s\n' '// TRACK: '"BLI-CEF-R15-ENV-TRUST-001 / REQ-CEF-R2-SEC-ENV-TRUST"
	printf '%s\n' 'func main() {}'
} >"$TRACK_REPO/cmd/zqk-shim/main.go"
git -C "$TRACK_REPO" init -q
git -C "$TRACK_REPO" add .
if sh "$TRACK_REPO/scripts/open-core/check-public-release-payload.sh" "$TRACK_REPO" >"$TRACK_REPO/result.log" 2>&1; then
	printf '%s\n' "payload gate accepted TRACK kernel id in cmd/zqk-shim" >&2
	exit 1
fi
grep -E 'studio TRACK comment with kernel object id remains in cmd, scripts, or pkg|studio TRACK comment with kernel object id remains' "$TRACK_REPO/result.log" >/dev/null

# Prove production cmd nanos-hex kernel ids fail closed (not only TRACK comments).
HEX_REPO=$(mktemp -d "${TMPDIR:-/tmp}/zqk-public-gate-hex.XXXXXX")
trap 'rm -f "$TMP_BIN"; rm -rf "$TMP_REPO" "$TRACK_REPO" "$HEX_REPO"' EXIT HUP INT TERM
mkdir -p "$HEX_REPO/cmd/zqk" "$HEX_REPO/config" "$HEX_REPO/docs/onboarding" "$HEX_REPO/scripts/open-core"
for path in README.md LICENSE NOTICE SECURITY.md CODE_OF_CONDUCT.md CONTRIBUTING.md; do
	printf '%s\n' "fixture" >"$HEX_REPO/$path"
done
printf '%s\n' 'module github.com/zqk-os/zqk' >"$HEX_REPO/go.mod"
printf '%s\n' 'brand:' '  executable_name: zqk' >"$HEX_REPO/config/zqk.yaml"
printf '%s\n' 'package main' >"$HEX_REPO/cmd/zqk/main.go"
printf '%s\n' 'fixture' >"$HEX_REPO/docs/INDEX.md"
printf '%s\n' 'fixture' >"$HEX_REPO/docs/onboarding/COMMUNITY_FIRST_RUN.md"
cp "$ROOT/scripts/open-core/check-public-release-payload.sh" "$HEX_REPO/scripts/open-core/"
cat >"$HEX_REPO/scripts/open-core/police-community-tree.sh" <<'EOF'
#!/bin/sh
exit 0
EOF
{
	printf '%s\n' 'package ci'
	printf '%s\n' '// Kernel: '"PRI-1785699924616992000-8000284f"
	printf '%s\n' 'func NewCICmd() {}'
} >"$HEX_REPO/cmd/zqk/ci.go"
git -C "$HEX_REPO" init -q
git -C "$HEX_REPO" add .
if sh "$HEX_REPO/scripts/open-core/check-public-release-payload.sh" "$HEX_REPO" >"$HEX_REPO/result.log" 2>&1; then
	printf '%s\n' "payload gate accepted nanos-hex kernel id in production cmd" >&2
	exit 1
fi
grep -E 'studio nanos-hex kernel object id remains in production (pkg, )?cmd' "$HEX_REPO/result.log" >/dev/null

# Prove studio process items in public documentation fail closed.
DOC_REPO=$(mktemp -d "${TMPDIR:-/tmp}/zqk-public-gate-doc.XXXXXX")
trap 'rm -f "$TMP_BIN"; rm -rf "$TMP_REPO" "$TRACK_REPO" "$HEX_REPO" "$DOC_REPO"' EXIT HUP INT TERM
mkdir -p "$DOC_REPO/cmd/zqk" "$DOC_REPO/config" "$DOC_REPO/docs/architecture" "$DOC_REPO/docs/onboarding" "$DOC_REPO/scripts/open-core"
for path in README.md LICENSE NOTICE SECURITY.md CODE_OF_CONDUCT.md CONTRIBUTING.md; do
	printf '%s\n' "fixture" >"$DOC_REPO/$path"
done
printf '%s\n' 'module github.com/zqk-os/zqk' >"$DOC_REPO/go.mod"
printf '%s\n' 'brand:' '  executable_name: zqk' >"$DOC_REPO/config/zqk.yaml"
printf '%s\n' 'package main' >"$DOC_REPO/cmd/zqk/main.go"
printf '%s\n' 'fixture' >"$DOC_REPO/docs/INDEX.md"
printf '%s\n' 'fixture' >"$DOC_REPO/docs/onboarding/COMMUNITY_FIRST_RUN.md"
cp "$ROOT/scripts/open-core/check-public-release-payload.sh" "$DOC_REPO/scripts/open-core/"
cat >"$DOC_REPO/scripts/open-core/police-community-tree.sh" <<'EOF'
#!/bin/sh
exit 0
EOF
{
	printf '%s\n' '# Taxonomy'
	printf '%s\n' '- **Priority Plan:** '"PRI-CLI-TAXONOMY-OVERHAUL-001"
} >"$DOC_REPO/docs/architecture/TAXONOMY.md"
git -C "$DOC_REPO" init -q
git -C "$DOC_REPO" add .
if sh "$DOC_REPO/scripts/open-core/check-public-release-payload.sh" "$DOC_REPO" >"$DOC_REPO/result.log" 2>&1; then
	printf '%s\n' "payload gate accepted Priority Plan in public documentation" >&2
	exit 1
fi
grep -F 'studio process item or governing lineage reference remains in public documentation' "$DOC_REPO/result.log" >/dev/null

# Prove tracked CEF run artifacts fail closed.
CEF_REPO=$(mktemp -d "${TMPDIR:-/tmp}/zqk-public-gate-cef.XXXXXX")
trap 'rm -f "$TMP_BIN"; rm -rf "$TMP_REPO" "$TRACK_REPO" "$HEX_REPO" "$DOC_REPO" "$CEF_REPO"' EXIT HUP INT TERM
mkdir -p "$CEF_REPO/cmd/zqk" "$CEF_REPO/config" "$CEF_REPO/docs/onboarding" "$CEF_REPO/docs/quality/cef-runs/2026-09-25-RUN" "$CEF_REPO/scripts/open-core"
for path in README.md LICENSE NOTICE SECURITY.md CODE_OF_CONDUCT.md CONTRIBUTING.md; do
	printf '%s\n' "fixture" >"$CEF_REPO/$path"
done
printf '%s\n' 'module github.com/zqk-os/zqk' >"$CEF_REPO/go.mod"
printf '%s\n' 'brand:' '  executable_name: zqk' >"$CEF_REPO/config/zqk.yaml"
printf '%s\n' 'package main' >"$CEF_REPO/cmd/zqk/main.go"
printf '%s\n' 'fixture' >"$CEF_REPO/docs/INDEX.md"
printf '%s\n' 'fixture' >"$CEF_REPO/docs/onboarding/COMMUNITY_FIRST_RUN.md"
cp "$ROOT/scripts/open-core/check-public-release-payload.sh" "$CEF_REPO/scripts/open-core/"
cat >"$CEF_REPO/scripts/open-core/police-community-tree.sh" <<'EOF'
#!/bin/sh
exit 0
EOF
printf '%s\n' '{"run_id":"test"}' >"$CEF_REPO/docs/quality/cef-runs/2026-09-25-RUN/scorecard.json"
git -C "$CEF_REPO" init -q
git -C "$CEF_REPO" add .
if sh "$CEF_REPO/scripts/open-core/check-public-release-payload.sh" "$CEF_REPO" >"$CEF_REPO/result.log" 2>&1; then
	printf '%s\n' "payload gate accepted tracked CEF run artifacts" >&2
	exit 1
fi
grep -F 'project-specific CEF run artifacts are tracked in git' "$CEF_REPO/result.log" >/dev/null

if [ "$MODE" = "payload-only" ]; then
	printf '%s\n' "PUBLIC PAYLOAD GATES: PASS"
	exit 0
fi

(
	cd "$ROOT"
	. "$ROOT/scripts/open-core/brand-env-prefix.sh"
	unset "${BRAND_ENV_PREFIX}_PROJECT_ROOT" || true

	# 1. Check hardcoded path and permission literals across repository
	if [ -x "$ROOT/scripts/check-hardcoded-paths-and-perms-repo.sh" ]; then
		"$ROOT/scripts/check-hardcoded-paths-and-perms-repo.sh" --no-dups
	fi

	# 2. Audit process file descriptor hygiene and verify absence of descriptor leaks (lsof gate)
	if [ -x "$ROOT/scripts/open-core/check-process-fd-leaks.sh" ]; then
		sh "$ROOT/scripts/open-core/check-process-fd-leaks.sh"
	fi

	# 2. Build entire project (all packages across cmd and pkg)
	CGO_ENABLED=0 go build -buildvcs=false ./...
	CGO_ENABLED=0 go build -buildvcs=false -o "$TMP_BIN" ./cmd/zqk
	export "${BRAND_ENV_PREFIX}_SHARED_TEST_BIN=$TMP_BIN"
	"$TMP_BIN" --help >/dev/null
	"$TMP_BIN" system --help >/dev/null
	"$TMP_BIN" test dashboard --help >/dev/null

	# 2. Run public CLI commands test suite
	go test -short -p 2 -timeout 5m \
		./cmd/zqk/app ./cmd/zqk/test ./cmd/zqk/docman ./cmd/zqk/automation \
		./cmd/zqk/new ./cmd/zqk/inbox ./cmd/zqk/learn ./cmd/zqk/matrix ./cmd/zqk/mcp ./cmd/zqk/mcp-simple \
		./cmd/zqk/mesh ./cmd/zqk/feed ./cmd/zqk/convergence ./cmd/zqk/callback ./cmd/zqk/intake \
		./cmd/zqk/domain ./cmd/zqk/ambient ./cmd/zqk/agent ./cmd/zqk/swarm ./cmd/zqk/spec ./cmd/zqk/keystore
	go test ./cmd/zqk/system -run 'TestInit_Greenfield$|TestInit_Legacy$|TestInit_Greenfield_StarterKernelGraph$|TestInit_Greenfield_NoEnvVars$|TestCheckOutput|TestSystemCheck|TestCommandSpecPolicingAudit|TestCLITaxonomyGovernance|TestRollupCLITaxonomyOverhaul|TestWriteProjectConfigFiles' -timeout 5m

	# 3. Run core kernel packages test suite
	go test -short -p 2 -timeout 5m \
		./pkg/brand/... ./pkg/bridge/... ./pkg/circuitbreaker/... ./pkg/cli/... \
		./pkg/concurrency/... ./pkg/dna/... ./pkg/docman/... ./pkg/graph/... \
		./pkg/healthcheck/... ./pkg/hive/... ./pkg/hostload/... ./pkg/integrity/... \
		./pkg/interactive/... ./pkg/kernel/... ./pkg/telemetry/... ./pkg/accumulator/... \
		./pkg/authcred/... ./pkg/bufferpool/... ./pkg/cleanup/... ./pkg/clihooks/... \
		./pkg/closureevidence/... ./pkg/coordination/... ./pkg/crypto/... ./pkg/datacell/... \
		./pkg/dispatch/... ./pkg/events/... ./pkg/grooming/... ./pkg/handslapper/... \
		./pkg/hivemind/... ./pkg/idebridge/... ./pkg/idehooks/... ./pkg/inbox/... \
		./pkg/infrastructure/... ./pkg/ingestion/... ./pkg/interactionpolicy/... \
		./pkg/kernelcas ./pkg/lifecycle/... ./pkg/lockhealth/... ./pkg/observability/... \
		./pkg/paths/... ./pkg/pipeline/... ./pkg/tray/... ./pkg/vds/... ./pkg/walutil/... \
		./pkg/workflow/whatsnext

	# 4. Run storage package test suite
	go test -short -p 4 -timeout 20m ./pkg/storage/...
)

printf '%s\n' "PUBLIC RELEASE GATES: PASS"
