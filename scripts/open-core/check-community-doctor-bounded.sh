#!/usr/bin/env bash
# check-community-doctor-bounded.sh — stranger feed doctor / status stays pack-free.
#
# Boots an isolated greenfield cell, runs wall-clock-bounded `feed doctor --format json`
# (and `system status --format json` when available), fail-closes unless:
#   1) doctor exits 0 within the wall-clock bound (no soft-fail swallow),
#   2) doctor JSON is structured (feed_health / issues / peer_wake_live / …),
#   3) payload has no Studio pack ontology leakage (excl. node_sovereignty diagnostics),
#   4) temp-epoch ambient is stopped so cleanup does not leave a leaked daemon.
#
# Usage:
#   sh scripts/open-core/check-community-doctor-bounded.sh [candidate-root]
#   ZQK_BIN=/path/to/zqk sh scripts/open-core/check-community-doctor-bounded.sh
#
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger doctor residual
# TRACK: BLI-1794414873240313500-8bf29b97 — doctor smoke honesty after archive/COMMS
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
	# Absolute path so chdir into the stranger cell cannot break relative ZQK_BIN.
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

say "# community doctor bounded (stranger pack-free honesty)"
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
PROJECT="$WORK/project"
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
		if "$bin" ambient stop --help >/dev/null 2>&1; then
			"$bin" ambient stop >/dev/null 2>&1 || true
		fi
	)
	pkill -f "$WORK" >/dev/null 2>&1 || true
	sleep 0.2
}

cleanup() {
	if [ "$CLEANED" = "1" ]; then
		return 0
	fi
	CLEANED=1
	stop_temp_ambient "$PROJECT" "$TEST_ZQK"
	chmod -R u+w "$WORK" >/dev/null 2>&1 || true
	rm -rf "$WORK" >/dev/null 2>&1 || true
}
trap cleanup EXIT

mkdir -p "$PROJECT" "$WORK/home"
DOCTOR_RC=0
STATUS_RC=0
(
	cd "$PROJECT"
	# Avoid studio kernel bleed-through into the stranger cell.
	# Isolated HOME so operator ~/.cursor/mcp.json (studio pin) cannot hijack
	# node_sovereignty and poison pack-deny scans with absolute studio paths.
	unset ZQK_PROJECT_ROOT ZQK_CURRENT_ROOT ZQK_ENABLE_STUDIO_PACK_TOOLS ZQK_STUDIO_DOGFOOD 2>/dev/null || true
	export HOME="$WORK/home"
	export ZQK_HOME="$WORK/home"
	if "$TEST_ZQK" init --help >/dev/null 2>&1; then
		run_bounded 45 "$TEST_ZQK" init >/dev/null
	else
		run_bounded 45 "$TEST_ZQK" system init >/dev/null
	fi
	# Honesty: feed doctor must succeed within the bound (no || true swallow).
	set +e
	run_bounded 45 "$TEST_ZQK" feed doctor --format json >"$WORK/doctor.json" 2>"$WORK/doctor.err"
	DOCTOR_RC=$?
	set -e
	printf '%s' "$DOCTOR_RC" >"$WORK/doctor.rc"
	if "$TEST_ZQK" system status --help >/dev/null 2>&1; then
		set +e
		run_bounded 45 "$TEST_ZQK" system status --format json >"$WORK/status.json" 2>"$WORK/status.err"
		STATUS_RC=$?
		set -e
		printf '%s' "$STATUS_RC" >"$WORK/status.rc"
	fi
)

DOCTOR_RC="$(cat "$WORK/doctor.rc" 2>/dev/null || echo 1)"
[ -s "$WORK/doctor.json" ] || fail "feed doctor produced empty output (rc=$DOCTOR_RC)"
[ "$DOCTOR_RC" = "0" ] || fail "feed doctor exited $DOCTOR_RC (bounded honesty; stderr=$(head -c 200 "$WORK/doctor.err" 2>/dev/null | tr '\n' ' '))"

# Status is optional capability, but when present must not soft-fail into empty JSON.
if [ -f "$WORK/status.rc" ]; then
	STATUS_RC="$(cat "$WORK/status.rc")"
	if [ "$STATUS_RC" = "0" ]; then
		[ -s "$WORK/status.json" ] || fail "system status exited 0 but produced empty output"
	else
		# Non-zero status is allowed only when no JSON was emitted (honest absence).
		[ ! -s "$WORK/status.json" ] || fail "system status exited $STATUS_RC but wrote JSON (soft-fail honesty)"
		rm -f "$WORK/status.json"
	fi
fi

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
# Honesty: require a real doctor diagnostic shape (not an empty/ unrelated object).
if not any(k in doctor for k in ("feed_health", "issues", "peer_wake_live", "mcp_subscribers", "contract_paths_ok", "default_seating_ok")):
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

# Sealed lead ids split so payload nanos-hex grep does not false-positive this gate.
sealed_pris = [
    "PRI-" + "1794414492536041000-532f5365",  # sealed archive residual
    "PRI-" + "1794413974482357000-9876ca42",  # sealed feed/COMMS residual
    "PRI-" + "1794408540878556000-de9a91f3",  # sealed prior doctor residual
    "PRI-" + "1794405067761890000-7b6d3831",  # sealed greenfield lead
    "PRI-" + "1794404844729744000-6a42dca7",  # sealed seed lead
]
deny = re.compile(
    r"(?i)"
    r"(PER-ORCH-|PER-STUDIO-|ASK-STUDIO-|ASK-CAP-|"
    r"mmorch|ORCHESTRATE_PLAN|"
    r"get_current_priority_plan|get_priority_plan_items|"
    r"get_current_backlog_item|get_next_backlog_item|"
    r"cap_orchestrat|cap_night_duty|cap_background|"
    r"paste_applescript|print_cursor_paste|print_ide_paste|zqk_cursor|"
    r"align-cache|align_cache|"
    + "|".join(re.escape(s) for s in sealed_pris)
    + r"|"
    r"zqke-pack-composition|/Users/lanceettl/zqke-pack-composition)"
)
hit = deny.search(blob)
if hit:
    print(f"FAIL: pack ontology leakage in doctor/status payload near {hit.group(0)!r}", file=sys.stderr)
    sys.exit(1)

print("PASS: bounded feed doctor (and status if present) pack-free; doctor exit=0")
PY

# Ambient honesty: stop temp daemon before asserting cleanup path is clear.
stop_temp_ambient "$PROJECT" "$TEST_ZQK"
if [ -f "$PROJECT/.zqk/state/ambient/daemon.pid" ]; then
	fail "temp ambient daemon.pid still present after ambient stop"
fi

say "COMMUNITY DOCTOR BOUNDED: PASS"
