#!/bin/sh
# Fail-closed source-tree gate. Passing does not authorize a public push.
set -eu

ROOT=${1:-}
if [ -z "$ROOT" ]; then
	ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)
FAIL=0

say() { printf '%s\n' "$*"; }
fail() {
	say "BLOCK: $*"
	FAIL=1
}

say "# public-release payload check"
say "root=$ROOT"

for path in README.md LICENSE NOTICE SECURITY.md CODE_OF_CONDUCT.md CONTRIBUTING.md \
	go.mod config/zqk.yaml cmd/zqk/main.go docs/INDEX.md docs/onboarding/COMMUNITY_FIRST_RUN.md \
	scripts/open-core/police-community-tree.sh; do
	if [ ! -f "$ROOT/$path" ]; then
		fail "required public artifact missing: $path"
	fi
done

for path in docs/_archive cmd/zqk-admin cmd/codegen_runner \
	cmd/pattern-cli pkg/billing \
	pkg/community/container_helm_test.go docs/commercial docs/marketing docs/launch; do
	if [ -e "$ROOT/$path" ]; then
		fail "forbidden or unconfigured public artifact present: $path"
	fi
done

if ! grep -Fqx 'module github.com/zqk-os/zqk' "$ROOT/go.mod"; then
	fail "go.mod does not declare module github.com/zqk-os/zqk"
fi

PAYLOAD_WORK=$(mktemp -d "${TMPDIR:-/tmp}/zqk-payload-check.XXXXXX")
trap 'rm -rf "$PAYLOAD_WORK" >/dev/null 2>&1 || true' EXIT HUP INT TERM

# Run independent git grep / ls-files scrubs concurrently to avoid sequential I/O stalls.
git -C "$ROOT" grep -n -F 'github.com/lanceman/zqk' -- '*.go' go.mod >"$PAYLOAD_WORK/g1.out" 2>&1 &
PID_G1=$!

# Packaging/operator scripts must not advertise the private module or GitHub identity.
# rewrite-community-module-path.sh is the one-shot migrator and may name the old path.
git -C "$ROOT" grep -n -F 'github.com/lanceman/zqk' -- \
	'scripts' \
	':!scripts/open-core/rewrite-community-module-path.sh' \
	':!scripts/open-core/check-public-release-payload.sh' >"$PAYLOAD_WORK/g2.out" 2>&1 &
PID_G2=$!

git -C "$ROOT" grep -n -F 'lanceman/zqk' -- \
	'scripts' \
	':!scripts/open-core/rewrite-community-module-path.sh' \
	':!scripts/open-core/check-public-release-payload.sh' >"$PAYLOAD_WORK/g3.out" 2>&1 &
PID_G3=$!

git -C "$ROOT" grep -n -i 'lanceman' -- \
	. \
	':!scripts/open-core/rewrite-community-module-path.sh' \
	':!scripts/open-core/check-public-release-payload.sh' \
	':!config/gates.yaml' >"$PAYLOAD_WORK/g4.out" 2>&1 &
PID_G4=$!

# Studio technical_debt instance IDs (TDE-<nanos>-<hex>) belong in kernel CAS,
# not in public overlay/packaging comments.
git -C "$ROOT" grep -n -E 'TDE-[0-9]{15,}-[0-9a-fA-F]{8}' -- \
	'scripts' \
	':!scripts/open-core/check-public-release-payload.sh' \
	':!scripts/open-core/test-public-release-gates.sh' \
	':!scripts/fixtures/evidence/**' \
	':!scripts/fixtures/objectify_claims/**' \
	':!scripts/fixtures/change_intents/**' >"$PAYLOAD_WORK/g5.out" 2>/dev/null &
PID_G5=$!

# TRACK comments must not leak studio kernel object ids (BLI/REQ/CRIT/PRI/TDE/ATK/CAP/CVS).
# cmd/zqk-shim previously shipped BLI-CEF-* / REQ-CEF-* TRACK lines while this gate
# only scanned TDE-nanos-hex under scripts/.
git -C "$ROOT" grep -n -E 'TRACK:.*(BLI|REQ|CRIT|PRI|TDE|ATK|CAP|CVS)-' -- \
	'cmd' 'scripts' 'pkg' \
	':!scripts/open-core/check-public-release-payload.sh' \
	':!scripts/open-core/test-public-release-gates.sh' \
	':!scripts/fixtures/evidence/**' \
	':!scripts/fixtures/objectify_claims/**' \
	':!scripts/fixtures/change_intents/**' \
	':!pkg/**/testdata/*' >"$PAYLOAD_WORK/g6.out" 2>/dev/null &
PID_G6=$!

# CEF run artifacts and evaluation reports are project-specific and must not be tracked in git
git -C "$ROOT" ls-files 'docs/quality/cef-runs' 'docs/eval' >"$PAYLOAD_WORK/g7.out" 2>/dev/null &
PID_G7=$!

# Studio CAS nanos-hex instance ids belong in kernel CAS, not production cmd/, docs/, scripts/,
# command DNA, or generated builders. Tests may still mint synthetic ids.
git -C "$ROOT" grep -n -E '[A-Z]{2,12}-[0-9]{15,}-[0-9a-fA-F]{8}' -- \
	'cmd' \
	'pkg' \
	'docs/architecture' \
	'docs/specs' \
	'docs/onboarding' \
	'docs/howto' \
	'docs/manual' \
	'docs/tutorials' \
	'docs/development' \
	'scripts' \
	'.zqk/cli/specs' \
	'pkg/cli/bldr_cli_cmd_v1' \
	':!*_test.go' \
	':!pkg/**/testdata/*' \
	':!scripts/open-core/check-public-release-payload.sh' \
	':!scripts/open-core/test-public-release-gates.sh' \
	':!scripts/fixtures/evidence/**' \
	':!scripts/fixtures/objectify_claims/**' \
	':!scripts/fixtures/change_intents/**' >"$PAYLOAD_WORK/g8.out" 2>/dev/null &
PID_G8=$!

# Public documentation must not reference studio/isolated project process items:
# Priority plans, Backlog items, Governing Goals, Governing Requirements, or nanosecond kernel IDs.
git -C "$ROOT" grep -n -E '(Priority Plan|Backlog Items? Verified|Governing (Goal|Requirement|Priority Plan|Backlog Item)):.*(PRI|BLI|REQ|CRIT|GOAL|WKS)-' -- \
	'docs' \
	':!scripts/open-core/check-public-release-payload.sh' \
	':!scripts/open-core/test-public-release-gates.sh' >"$PAYLOAD_WORK/g9.out" 2>/dev/null &
PID_G9=$!

# Isolated project-specific process items (e.g. PRI-CLI-*, BLI-CLI-*, PRI-IO-*, BLI-IO-*, etc.)
# minted in isolated private context must not appear in public documentation.
git -C "$ROOT" grep -n -E '\b(PRI|BLI)-(CLI|IO|ERGONOMICS|OBJECT-INSPECT|LAUNCH)-[A-Z0-9_-]+' -- \
	'docs/architecture' \
	'docs/specs' \
	'docs/onboarding' \
	'docs/howto' \
	'docs/manual' \
	'docs/tutorials' \
	'docs/development' \
	':!docs/manual/screenshots/*.svg' \
	':!scripts/open-core/check-public-release-payload.sh' \
	':!scripts/open-core/test-public-release-gates.sh' >"$PAYLOAD_WORK/g10.out" 2>/dev/null &
PID_G10=$!

# CEF program ids are studio-only; keep them out of shipped cmd/scripts (not tests).
git -C "$ROOT" grep -n -E '(BLI|REQ|CRIT|TDE)-CEF-' -- \
	'cmd' 'scripts' \
	':!*_test.go' \
	':!scripts/open-core/check-public-release-payload.sh' \
	':!scripts/open-core/test-public-release-gates.sh' >"$PAYLOAD_WORK/g11.out" 2>/dev/null &
PID_G11=$!

git -C "$ROOT" grep -n -E 'Traceability:.*(BLI|REQ|CRIT)-' -- 'cmd' 'docs' >"$PAYLOAD_WORK/g12.out" 2>/dev/null &
PID_G12=$!

git -C "$ROOT" grep -n -E '\bzcom\b|zqk-community' -- \
	README.md CONTRIBUTING.md ZQK_GETTING_STARTED.md \
	'docs/onboarding/*.md' 'docs/architecture/*.md' \
	'scripts/open-core/sku-overlay/*.md' >"$PAYLOAD_WORK/g13.out" 2>&1 &
PID_G13=$!

git -C "$ROOT" ls-files \
	'.zqk/process/**' 'docs/process/**' \
	'.zqk/keystore/**' '.zqk/state/**' 'config/zqk-local.yaml' \
	'*.pem' '*.key' '*.p12' '*.pfx' '*.test' >"$PAYLOAD_WORK/g14.out" 2>/dev/null &
PID_G14=$!

git -C "$ROOT" grep -l -E \
	'BEGIN (RSA |EC |OPENSSH )?PRIVATE KEY|AKIA[0-9A-Z]{16}|ghp_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{40,}|xox[baprs]-[A-Za-z0-9-]{10,}' \
	-- . ':!scripts/open-core/check-public-release-payload.sh' >"$PAYLOAD_WORK/g15.out" 2>&1 &
PID_G15=$!

# police-community-tree.sh also runs this payload scrub; the guard breaks the
# payload -> police -> payload mutual recursion (fork-bomb) in either entry order.
PID_POLICE=""
if [ "${ZQK_PAYLOAD_POLICE_GUARD:-}" != "police" ]; then
	ZQK_PAYLOAD_POLICE_GUARD=payload sh "$ROOT/scripts/open-core/police-community-tree.sh" "$ROOT" >"$PAYLOAD_WORK/police.out" 2>&1 &
	PID_POLICE=$!
fi

wait_match_fail() {
	_pid=$1
	_out=$2
	_print=$3
	_msg=$4
	set +e
	wait "$_pid"
	_rc=$?
	set -e
	if [ "$_rc" -eq 0 ]; then
		if [ "$_print" = "1" ] && [ -s "$_out" ]; then
			cat "$_out"
		fi
		fail "$_msg"
	fi
}

wait_match_fail "$PID_G1" "$PAYLOAD_WORK/g1.out" 0 "private module path remains in Go source or go.mod"
wait_match_fail "$PID_G2" "$PAYLOAD_WORK/g2.out" 0 "private module path remains in scripts"
wait_match_fail "$PID_G3" "$PAYLOAD_WORK/g3.out" 0 "private GitHub identity remains in scripts"
wait_match_fail "$PID_G4" "$PAYLOAD_WORK/g4.out" 0 "private owner/namespace reference (lanceman) remains in tracked repo files"
wait_match_fail "$PID_G5" "$PAYLOAD_WORK/g5.out" 1 "studio technical_debt instance id remains in scripts"
wait_match_fail "$PID_G6" "$PAYLOAD_WORK/g6.out" 1 "studio TRACK comment with kernel object id remains in cmd, scripts, or pkg"

set +e
wait "$PID_G7"
set -e
if [ -s "$PAYLOAD_WORK/g7.out" ]; then
	fail "project-specific CEF run artifacts or evaluation reports are tracked in git"
fi

wait_match_fail "$PID_G8" "$PAYLOAD_WORK/g8.out" 1 "studio nanos-hex kernel object id remains in production pkg, cmd, docs, scripts, CLI specs, or command builders"
wait_match_fail "$PID_G9" "$PAYLOAD_WORK/g9.out" 1 "studio process item or governing lineage reference remains in public documentation"
wait_match_fail "$PID_G10" "$PAYLOAD_WORK/g10.out" 1 "isolated project-specific process item remains in public documentation"
wait_match_fail "$PID_G11" "$PAYLOAD_WORK/g11.out" 1 "studio CEF kernel object id remains in production cmd or scripts"
wait_match_fail "$PID_G12" "$PAYLOAD_WORK/g12.out" 1 "studio Traceability header with kernel object id remains in cmd or docs"

if grep -E 'APPENDIX A|Proprietary|Commercial Enterprise|OPEN_CORE_PROPRIETARY_SPLIT' \
	"$ROOT/LICENSE" "$ROOT/NOTICE" >/dev/null 2>&1; then
	fail "LICENSE or NOTICE still carries the studio monorepo carve-out"
fi

wait_match_fail "$PID_G13" "$PAYLOAD_WORK/g13.out" 0 "public first-run text contains a local executable or compile-unit name"

set +e
wait "$PID_G14"
set -e
if [ -s "$PAYLOAD_WORK/g14.out" ]; then
	fail "tracked private/runtime material found"
	cat "$PAYLOAD_WORK/g14.out"
fi

wait_match_fail "$PID_G15" "$PAYLOAD_WORK/g15.out" 0 "tracked files contain an obvious credential or private-key marker"

if [ "${ZQK_PAYLOAD_POLICE_GUARD:-}" = "police" ]; then
	say "SKIP: community tree police (already running as parent of this payload scrub)"
elif [ -n "$PID_POLICE" ]; then
	set +e
	wait "$PID_POLICE"
	_prc=$?
	set -e
	cat "$PAYLOAD_WORK/police.out"
	if [ "$_prc" -ne 0 ]; then
		fail "community tree police failed"
	fi
fi

# Explicit seed impermeability (also invoked from police); keep as a named gate for CI clarity.
if [ -f "$ROOT/scripts/open-core/check-bootstrap-seed-impermeability.sh" ]; then
	if ! sh "$ROOT/scripts/open-core/check-bootstrap-seed-impermeability.sh" "$ROOT"; then
		fail "bootstrap seed impermeability failed (CAP/MMORCH/orchestrate/TPM in init archive)"
	fi
else
	fail "missing scripts/open-core/check-bootstrap-seed-impermeability.sh"
fi

if [ "$FAIL" -ne 0 ]; then
	say "RESULT=FAIL (do not publish)"
	exit 1
fi

say "RESULT=PASS (payload only; human public-push acknowledgment is still required)"
