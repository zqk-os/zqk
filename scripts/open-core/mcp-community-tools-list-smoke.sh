#!/usr/bin/env bash
# Community MCP tools/list parity smoke (stranger profile).
# Fail-closed if pack ontology getters appear in list-tools output.
# Docs: docs/onboarding/CURSOR_MCP_SETUP.md
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger MCP tools/list must exclude pack ontology getters
set -euo pipefail

ROOT="${ZQK_COMMUNITY_ROOT:-}"
if [[ -z "$ROOT" ]]; then
  if [[ -x /Users/lanceettl/zqk-public-candidate/bin/zqk ]]; then
    ROOT=/Users/lanceettl/zqk-public-candidate
  elif [[ -x ./bin/zqk ]]; then
    ROOT=$(pwd)
  else
    echo "FAIL: set ZQK_COMMUNITY_ROOT to a community checkout with bin/zqk" >&2
    exit 2
  fi
fi

ZQK="$ROOT/bin/zqk"
if [[ ! -x "$ZQK" ]]; then
  echo "FAIL: missing executable $ZQK" >&2
  exit 2
fi

# Pack ontology getters that must NOT appear on stranger default MCP.
PACK_DENY_REGEX='zqk_get_current_priority_plan|zqk_get_priority_plan_items|zqk_get_current_backlog_item|zqk_get_next_backlog_item'

OUT=$(mktemp)
trap 'rm -f "$OUT"' EXIT

echo "mcp-community-tools-list-smoke: root=$ROOT"
(cd "$ROOT" && "$ZQK" mcp list-tools 2>/dev/null | tee "$OUT") || {
  echo "FAIL: mcp list-tools exited non-zero" >&2
  exit 1
}

if ! grep -q 'zqk_' "$OUT"; then
  echo "FAIL: no zqk_ tools listed (empty surface)" >&2
  exit 1
fi

# Required community discovery / health tools (minimum parity with Cursor docs).
for need in zqk_system_status zqk_object_list zqk_object_get; do
  if ! grep -Fq "$need" "$OUT"; then
    echo "FAIL: required tool missing: $need" >&2
    exit 1
  fi
done

HITS=$(grep -E "$PACK_DENY_REGEX" "$OUT" || true)
if [[ -n "$HITS" ]]; then
  echo "FAIL: pack ontology MCP tools still exposed on community/stranger surface:" >&2
  echo "$HITS" >&2
  echo "See KERNEL_VS_PACK_INVENTORY.md + CURSOR_MCP_SETUP.md" >&2
  exit 1
fi

echo "PASS: community tools/list has required tools and no pack ontology getters"
