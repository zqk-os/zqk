#!/usr/bin/env bash
# check-community-doctor-bounded.sh — stranger feed doctor / status stays pack-free.
#
# Boots an isolated greenfield cell, runs wall-clock-bounded `feed doctor --format json`
# (and `system status --format json` when available), fail-closes unless:
#   1) doctor JSON is structured,
#   2) payload has no Studio pack ontology leakage (excl. node_sovereignty diagnostics).
#
# Usage:
#   sh scripts/open-core/check-community-doctor-bounded.sh [candidate-root]
#   ZQK_BIN=/path/to/zqk sh scripts/open-core/check-community-doctor-bounded.sh
#
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger doctor residual
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

say "# community doctor bounded (stranger pack-free)"
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

WORK="$(mktemp -d "${TMPDIR:-/tmp}/zqk-doctor-bounded.XXXXXX")"
cleanup() { rm -rf "$WORK" >/dev/null 2>&1 || true; }
trap cleanup EXIT

mkdir -p "$WORK/project" "$WORK/home"
(
	cd "$WORK/project"
	unset ZQK_PROJECT_ROOT ZQK_CURRENT_ROOT ZQK_ENABLE_STUDIO_PACK_TOOLS ZQK_STUDIO_DOGFOOD 2>/dev/null || true
	export HOME="$WORK/home"
	export ZQK_HOME="$WORK/home"
	if "$TEST_ZQK" init --help >/dev/null 2>&1; then
		run_bounded 45 "$TEST_ZQK" init >/dev/null
	else
		run_bounded 45 "$TEST_ZQK" system init >/dev/null
	fi
	run_bounded 45 "$TEST_ZQK" feed doctor --format json >"$WORK/doctor.json" || true
	if "$TEST_ZQK" system status --help >/dev/null 2>&1; then
		run_bounded 45 "$TEST_ZQK" system status --format json >"$WORK/status.json" || true
	fi
)

[ -s "$WORK/doctor.json" ] || fail "feed doctor produced empty output"

python3 - "$WORK/doctor.json" "${WORK}/status.json" <<'PY'
import json, re, sys
from pathlib import Path

def load(path):
    p = Path(path)
    if not p.is_file() or p.stat().st_size == 0:
        return None
    return json.loads(p.read_text(encoding="utf-8"))

doctor = load(sys.argv[1])
if not isinstance(doctor, dict):
    print("FAIL: feed doctor output is not a JSON object", file=sys.stderr)
    sys.exit(1)
if not any(k in doctor for k in ("channel", "issues", "peer_wake_live", "mcp", "status", "ok")):
    # Accept any structured diagnostic with nested maps
    if not any(isinstance(v, (dict, list, bool, str, int)) for v in doctor.values()):
        print("FAIL: feed doctor JSON missing structured diagnostic fields", file=sys.stderr)
        sys.exit(1)

scrub = dict(doctor)
# Operator-host diagnostics are not product ontology.
for k in ("node_sovereignty", "kernel_ambience"):
    scrub.pop(k, None)
blob = json.dumps(scrub, sort_keys=True)

status = load(sys.argv[2]) if len(sys.argv) > 2 else None
if isinstance(status, dict):
    st = dict(status)
    st.pop("node_sovereignty", None)
    blob += json.dumps(st, sort_keys=True)

deny = re.compile(
    r"(?i)"
    r"(PER-ORCH-|PER-STUDIO-|ASK-STUDIO-|ASK-CAP-|"
    r"mmorch|ORCHESTRATE_PLAN|"
    r"get_current_priority_plan|get_priority_plan_items|"
    r"get_current_backlog_item|get_next_backlog_item|"
    r"cap_orchestrat|cap_night_duty|cap_background|"
    r"paste_applescript|print_cursor_paste|print_ide_paste|zqk_cursor|"
    r"align-cache|align_cache|"
    r"zqke-pack-composition|/Users/lanceettl/zqke-pack-composition)"
)
hit = deny.search(blob)
if hit:
    print(f"FAIL: pack ontology leakage in doctor/status payload near {hit.group(0)!r}", file=sys.stderr)
    sys.exit(1)

print("PASS: bounded feed doctor (and status if present) pack-free")
PY

say "COMMUNITY DOCTOR BOUNDED: PASS"
