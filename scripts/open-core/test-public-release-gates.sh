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
: >"$TMP_REPO/.goreleaser.yaml"
git -C "$TMP_REPO" init -q
git -C "$TMP_REPO" add .
if sh "$TMP_REPO/scripts/open-core/check-public-release-payload.sh" "$TMP_REPO" >"$TMP_REPO/result.log" 2>&1; then
	printf '%s\n' "payload gate accepted forbidden .goreleaser.yaml" >&2
	exit 1
fi
grep -F 'forbidden or unconfigured public artifact present: .goreleaser.yaml' "$TMP_REPO/result.log" >/dev/null

if [ "$MODE" = "payload-only" ]; then
	printf '%s\n' "PUBLIC PAYLOAD GATES: PASS"
	exit 0
fi

(
	cd "$ROOT"
	unset ZQK_PROJECT_ROOT
	go test ./cmd/zqk/app ./cmd/zqk/system ./cmd/zqk/test -timeout 10m
	CGO_ENABLED=0 go build -buildvcs=false -o "$TMP_BIN" ./cmd/zqk
	"$TMP_BIN" --help >/dev/null
	"$TMP_BIN" system --help >/dev/null
	"$TMP_BIN" test dashboard --help >/dev/null
)

printf '%s\n' "PUBLIC RELEASE GATES: PASS"
