#!/usr/bin/env bash
# Community MCP tools/list parity smoke (stranger profile).
# Fail-closed if pack ontology getters appear in list-tools output.
# Docs: docs/onboarding/CURSOR_MCP_SETUP.md
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger MCP tools/list must exclude pack ontology getters
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="${1:-}"
if [ -z "$ROOT" ]; then
	ROOT="${ZQK_COMMUNITY_ROOT:-}"
fi
if [ -z "$ROOT" ]; then
	ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
else
	ROOT="$(CDPATH= cd -- "$ROOT" && pwd)"
fi

say() { printf '%s\n' "$*"; }
fail() { say "FAIL: $*" >&2; exit 1; }

resolve_bin() {
	local cand="$1"
	[ -n "$cand" ] || return 1
	[ -x "$cand" ] || return 1
	(CDPATH= cd -- "$(dirname "$cand")" && printf '%s/%s\n' "$(pwd)" "$(basename "$cand")")
}

ZQK=""
if [ -n "${ZQK_BIN:-}" ]; then
	ZQK="$(resolve_bin "$ZQK_BIN" || true)"
fi
if [ -z "$ZQK" ] && [ -x "$ROOT/bin/zqk" ]; then
	ZQK="$(resolve_bin "$ROOT/bin/zqk")"
fi
[ -n "$ZQK" ] || fail "no community zqk binary (set ZQK_BIN or pass candidate-root with bin/zqk)"

# Pack ontology getters that must NOT appear on stranger default MCP.
PACK_DENY_REGEX='zqk_get_current_priority_plan|zqk_get_priority_plan_items|zqk_get_current_backlog_item|zqk_get_next_backlog_item'

WORK="$(mktemp -d "${TMPDIR:-/tmp}/zqk-mcp-tools-list.XXXXXX")"
OUT="$WORK/tools.out"
cleanup() {
	if [ -x "$ZQK" ] && [ -d "$WORK/project" ]; then
		(
			cd "$WORK/project"
			unset ZQK_CURRENT_ROOT 2>/dev/null || true
			export ZQK_PROJECT_ROOT="$WORK/project"
			"$ZQK" ambient stop >/dev/null 2>&1 || true
		)
	fi
	pkill -f "$WORK" >/dev/null 2>&1 || true
	rm -rf "$WORK" >/dev/null 2>&1 || true
}
trap cleanup EXIT

mkdir -p "$WORK/project" "$WORK/home"
say "mcp-community-tools-list-smoke: root=$ROOT binary=$ZQK"

(
	cd "$WORK/project"
	unset ZQK_PROJECT_ROOT ZQK_CURRENT_ROOT ZQK_ENABLE_STUDIO_PACK_TOOLS ZQK_STUDIO_DOGFOOD 2>/dev/null || true
	export HOME="$WORK/home"
	export ZQK_HOME="$WORK/home"
	# Init a stranger cell so MCP list reflects community defaults, not operator studio pins.
	if "$ZQK" init --help >/dev/null 2>&1; then
		"$ZQK" init >/dev/null 2>&1 || true
	else
		"$ZQK" system init >/dev/null 2>&1 || true
	fi
	"$ZQK" mcp list-tools >"$OUT" 2>/dev/null || fail "mcp list-tools exited non-zero"
)

[ -s "$OUT" ] || fail "mcp list-tools produced empty output"
if ! grep -q 'zqk_' "$OUT"; then
	fail "no zqk_ tools listed (empty surface)"
fi

# Required community discovery / health tools (minimum parity with Cursor docs).
for need in zqk_system_status zqk_object_list zqk_object_get; do
	if ! grep -Fq "$need" "$OUT"; then
		fail "required tool missing: $need"
	fi
done

HITS=$(grep -E "$PACK_DENY_REGEX" "$OUT" || true)
if [ -n "$HITS" ]; then
	say "FAIL: pack ontology MCP tools still exposed on community/stranger surface:" >&2
	say "$HITS" >&2
	say "See KERNEL_VS_PACK_INVENTORY.md + CURSOR_MCP_SETUP.md" >&2
	exit 1
fi

# Absolute studio checkout paths must not appear in stranger tools/list text.
if grep -E 'zqke-pack-composition' "$OUT" >/dev/null 2>&1; then
	fail "studio checkout path leaked into mcp list-tools output"
fi

say "PASS: community tools/list has required tools and no pack ontology getters"
