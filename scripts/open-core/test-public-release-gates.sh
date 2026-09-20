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

# Prove studio technical_debt instance IDs in overlay scripts fail closed.
git -C "$TMP_REPO" rm -f -- cmd/zqk-admin >/dev/null
printf '%s\n' '# leftover studio id TDE-1789678536875854000-47240146' >"$TMP_REPO/scripts/open-core/applybrand-comment.sh"
git -C "$TMP_REPO" add -- scripts/open-core/applybrand-comment.sh
if sh "$TMP_REPO/scripts/open-core/check-public-release-payload.sh" "$TMP_REPO" >"$TMP_REPO/tde.log" 2>&1; then
	printf '%s\n' "payload gate accepted studio technical_debt instance id in scripts" >&2
	exit 1
fi
grep -F 'studio technical_debt instance id remains in scripts' "$TMP_REPO/tde.log" >/dev/null

if [ "$MODE" = "payload-only" ]; then
	printf '%s\n' "PUBLIC PAYLOAD GATES: PASS"
	exit 0
fi

(
	cd "$ROOT"
	unset ZQK_PROJECT_ROOT
	export ZQK_ALLOW_FOREGROUND_GO_TEST=1

	# 1. Build entire project across all cmd and pkg packages
	CGO_ENABLED=0 go build -buildvcs=false ./...
	CGO_ENABLED=0 go build -buildvcs=false -o "$TMP_BIN" ./cmd/zqk
	"$TMP_BIN" --help >/dev/null
	"$TMP_BIN" system --help >/dev/null
	"$TMP_BIN" test dashboard --help >/dev/null

	# 2. Run public CLI commands test suite
	go test -short -timeout 5m \
		./cmd/zqk/app ./cmd/zqk/test ./cmd/zqk/docman ./cmd/zqk/automation \
		./cmd/zqk/new ./cmd/zqk/inbox ./cmd/zqk/learn ./cmd/zqk/matrix ./cmd/zqk/mcp ./cmd/zqk/mcp-simple \
		./cmd/zqk/mesh ./cmd/zqk/feed ./cmd/zqk/convergence ./cmd/zqk/callback ./cmd/zqk/intake \
		./cmd/zqk/domain ./cmd/zqk/ambient
	go test ./cmd/zqk/system -run 'TestInit_Greenfield$|TestInit_Legacy$|TestInit_Greenfield_StarterKernelGraph$|TestInit_Greenfield_NoEnvVars$|TestCheckOutput|TestSystemCheck' -timeout 5m

	# 3. Run core kernel packages test suite
	go test -short -timeout 5m \
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
)

printf '%s\n' "PUBLIC RELEASE GATES: PASS"
