#!/usr/bin/env bash
# check-community-whats-next-bounded.sh — stranger whats-next stays pack-free.
#
# Boots an isolated greenfield cell with a community-capable binary, runs a
# wall-clock-bounded `workflow whats-next --skip-measure`, and fail-closes unless:
#   1) JSON is structured (zqk_whats_next_v1 / schema),
#   2) lead priority_plan is PRI-STARTER-COMMUNITY-001,
#   3) payload has no Studio pack ontology leakage.
#
# Usage:
#   sh scripts/open-core/check-community-whats-next-bounded.sh [candidate-root]
#   ZQK_BIN=/path/to/zqk sh scripts/open-core/check-community-whats-next-bounded.sh
#
# TRACK: BLI-1794405894395749000-cdae1049 — stranger whats-next residual
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

say "# community whats-next bounded (stranger pack-free)"
say "root=$ROOT binary=$TEST_ZQK"

# Bound greenfield init/whats-next so the gate cannot hang (macOS: perl alarm).
run_bounded() {
	_secs=$1
	shift
	if command -v perl >/dev/null 2>&1; then
		perl -e 'alarm int(shift); exec @ARGV' -- "$_secs" "$@"
	else
		"$@"
	fi
}

WORK="$(mktemp -d "${TMPDIR:-/tmp}/zqk-wn-bounded.XXXXXX")"
cleanup() { rm -rf "$WORK" >/dev/null 2>&1 || true; }
trap cleanup EXIT

mkdir -p "$WORK/project" "$WORK/home"
(
	cd "$WORK/project"
	# Avoid studio kernel bleed-through into the stranger cell.
	# Isolated HOME so operator ~/.cursor/mcp.json (studio pin) cannot hijack
	# node_sovereignty and poison pack-deny scans with absolute studio paths.
	# TRACK: BLI-1794405894395749000-cdae1049
	unset ZQK_PROJECT_ROOT ZQK_CURRENT_ROOT ZQK_ENABLE_STUDIO_PACK_TOOLS ZQK_STUDIO_DOGFOOD 2>/dev/null || true
	export HOME="$WORK/home"
	export ZQK_HOME="$WORK/home"
	if "$TEST_ZQK" init --help >/dev/null 2>&1; then
		run_bounded 45 "$TEST_ZQK" init >/dev/null
	else
		run_bounded 45 "$TEST_ZQK" system init >/dev/null
	fi
	run_bounded 45 "$TEST_ZQK" workflow whats-next --format json --skip-measure >"$WORK/whats-next.json"
)

[ -s "$WORK/whats-next.json" ] || fail "whats-next produced empty output"

python3 - "$WORK/whats-next.json" <<'PY'
import json, re, sys

path = sys.argv[1]
with open(path, encoding="utf-8") as f:
    raw = f.read()
try:
    d = json.loads(raw)
except json.JSONDecodeError as e:
    print(f"FAIL: whats-next output is not JSON: {e}", file=sys.stderr)
    sys.exit(1)

schema = str(d.get("schema") or d.get("schema_id") or "")
if "zqk_whats_next_v1" not in schema and '"schema"' not in raw and "zqk_whats_next_v1" not in json.dumps(d):
    # Accept either explicit schema field or known payload markers used by community probes.
    if not any(k in d for k in ("priority_plan", "guiding_step", "correspondence", "backlog")):
        print("FAIL: whats-next JSON missing structured kernel context", file=sys.stderr)
        sys.exit(1)

pp = d.get("priority_plan") or {}
pid = pp.get("id") if isinstance(pp, dict) else pp
want = "PRI-STARTER-COMMUNITY-001"
if pid != want:
    print(f"FAIL: expected priority_plan {want}, got {pid!r}", file=sys.stderr)
    sys.exit(1)

# Operator-host diagnostics (mcp_pin / node_sovereignty paths) are not product ontology.
scrub = dict(d)
ka = scrub.get("kernel_ambience")
if isinstance(ka, dict):
    ka = dict(ka)
    ka.pop("node_sovereignty", None)
    scrub["kernel_ambience"] = ka
blob = json.dumps(scrub, sort_keys=True)

# Studio / pack ontology that must not appear in stranger whats-next payloads.
deny = re.compile(
    r"(?i)"
    r"(PER-ORCH-|PER-STUDIO-|ASK-STUDIO-|ASK-CAP-|"
    r"mmorch|ORCHESTRATE_PLAN|"
    r"get_current_priority_plan|get_priority_plan_items|"
    r"get_current_backlog_item|get_next_backlog_item|"
    r"cap_orchestrat|cap_night_duty|cap_background|"
    r"paste_applescript|print_cursor_paste|print_ide_paste|zqk_cursor|"
    r"align-cache|align_cache|"
    r"PRI-1794405067761890000-7b6d3831|"  # sealed greenfield lead
    r"PRI-1794404844729744000-6a42dca7|"  # sealed seed lead
    r"PRI-1794405488622492000-90a0a917|"  # sealed MCP-ensure lead
    r"zqke-pack-composition|/Users/lanceettl/zqke-pack-composition)"
)
hit = deny.search(blob)
if hit:
    print(f"FAIL: pack ontology leakage in whats-next payload near {hit.group(0)!r}", file=sys.stderr)
    sys.exit(1)

print(f"PASS: bounded whats-next lead is {want} (pack-free)")
PY

say "COMMUNITY WHATS-NEXT BOUNDED: PASS"
