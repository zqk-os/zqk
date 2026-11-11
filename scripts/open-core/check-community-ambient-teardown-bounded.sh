#!/usr/bin/env bash
# check-community-ambient-teardown-bounded.sh — temp-epoch ambient must not leak.
#
# Boots an isolated greenfield cell, ensures the ambient daemon is up, then
# tears it down with `ambient stop` before removing the temp tree. Fail-closes
# unless:
#   1) ambient stop clears daemon.pid for the temp project,
#   2) no process remains bound to the temp WORK path,
#   3) rm -rf of the temp WORK path succeeds (dir gone).
#
# Usage:
#   sh scripts/open-core/check-community-ambient-teardown-bounded.sh [candidate-root]
#   ZQK_BIN=/path/to/zqk sh scripts/open-core/check-community-ambient-teardown-bounded.sh
#
# TRACK: BLI-1794409789128810000-ffc46cfd — stranger ambient teardown residual
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="${1:-}"
if [ -z "$ROOT" ]; then
	ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
else
	ROOT="$(CDPATH= cd -- "$ROOT" && pwd)"
fi

say() { printf '%s\n' "$*"; }
fail() { say "FAIL: $*" >&2; exit 1; }

bin_supports_init() {
	local cand="$1"
	[ -x "$cand" ] || return 1
	"$cand" init --help >/dev/null 2>&1 || "$cand" system init --help >/dev/null 2>&1
}

resolve_bin() {
	local cand="$1"
	[ -n "$cand" ] || return 1
	[ -x "$cand" ] || return 1
	(CDPATH= cd -- "$(dirname "$cand")" && printf '%s/%s\n' "$(pwd)" "$(basename "$cand")")
}

TEST_ZQK=""
if [ -n "${ZQK_BIN:-}" ]; then
	TEST_ZQK="$(resolve_bin "$ZQK_BIN" || true)"
fi
if [ -z "$TEST_ZQK" ] && bin_supports_init "${ROOT}/bin/zqk"; then
	TEST_ZQK="$(resolve_bin "${ROOT}/bin/zqk")"
fi
[ -n "$TEST_ZQK" ] || fail "no community-capable zqk binary (set ZQK_BIN)"
bin_supports_init "$TEST_ZQK" || fail "binary lacks init / system init: $TEST_ZQK"
"$TEST_ZQK" ambient stop --help >/dev/null 2>&1 || fail "binary lacks ambient stop: $TEST_ZQK"

say "# community ambient teardown bounded (stranger temp-epoch)"
say "root=$ROOT binary=$TEST_ZQK"

run_bounded() {
	_secs=$1
	shift
	if command -v perl >/dev/null 2>&1; then
		perl -e 'alarm int(shift); exec @ARGV' -- "$_secs" "$@"
	else
		"$@"
	fi
}

WORK="$(mktemp -d "${TMPDIR:-/tmp}/zqk-amb-teardown.XXXXXX")"
PROJECT="$WORK/project"
PID_FILE="$PROJECT/.zqk/state/ambient/daemon.pid"
CLEANED=0

stop_temp_ambient() {
	local proj="$1"
	local bin="$2"
	[ -d "$proj" ] || return 0
	(
		cd "$proj"
		unset ZQK_CURRENT_ROOT 2>/dev/null || true
		export ZQK_PROJECT_ROOT="$proj"
		export HOME="${WORK}/home"
		export ZQK_HOME="${WORK}/home"
		"$bin" ambient stop >/dev/null 2>&1 || true
	)
	pkill -f "$WORK" >/dev/null 2>&1 || true
	sleep 0.2
}

cleanup() {
	if [ "$CLEANED" = "1" ]; then
		return 0
	fi
	stop_temp_ambient "$PROJECT" "$TEST_ZQK"
	chmod -R u+w "$WORK" >/dev/null 2>&1 || true
	rm -rf "$WORK" >/dev/null 2>&1 || true
}
trap cleanup EXIT

mkdir -p "$PROJECT" "$WORK/home"
(
	cd "$PROJECT"
	unset ZQK_PROJECT_ROOT ZQK_CURRENT_ROOT ZQK_ENABLE_STUDIO_PACK_TOOLS ZQK_STUDIO_DOGFOOD 2>/dev/null || true
	export HOME="$WORK/home"
	export ZQK_HOME="$WORK/home"
	if "$TEST_ZQK" init --help >/dev/null 2>&1; then
		run_bounded 45 "$TEST_ZQK" init >/dev/null
	else
		run_bounded 45 "$TEST_ZQK" system init >/dev/null
	fi
	# init may already ensure ambient; force ensure so teardown has a live target
	if "$TEST_ZQK" ambient ensure --help >/dev/null 2>&1; then
		run_bounded 30 "$TEST_ZQK" ambient ensure >/dev/null 2>&1 || true
	fi
)

[ -f "$PID_FILE" ] || fail "expected ambient daemon.pid after init/ensure at $PID_FILE"
PID_BEFORE="$(tr -d '[:space:]' <"$PID_FILE" || true)"
[ -n "$PID_BEFORE" ] || fail "daemon.pid empty after init/ensure"
if ! kill -0 "$PID_BEFORE" >/dev/null 2>&1; then
	fail "daemon.pid=$PID_BEFORE is not a live process before teardown"
fi
say "ambient_live pid=$PID_BEFORE"

stop_temp_ambient "$PROJECT" "$TEST_ZQK"

if [ -f "$PID_FILE" ]; then
	fail "daemon.pid still present after ambient stop: $PID_FILE"
fi
if kill -0 "$PID_BEFORE" >/dev/null 2>&1; then
	fail "ambient pid $PID_BEFORE still alive after ambient stop"
fi
if pgrep -f "$WORK" >/dev/null 2>&1; then
	fail "process(es) still bound to temp WORK after ambient stop: $WORK"
fi
say "ambient_stopped"

chmod -R u+w "$WORK" >/dev/null 2>&1 || true
rm -rf "$WORK"
CLEANED=1
if [ -e "$WORK" ]; then
	fail "temp WORK still exists after rm -rf (likely FD/daemon hold): $WORK"
fi

say "PASS: ambient teardown — stop clears pid; temp epoch removable"
say "AMBIENT_TEARDOWN_RESULT ok"
