#!/usr/bin/env bash
# Community CLI help / command-discovery smoke (stranger profile).
# Fail-closed if pack ontology commands appear in root help or command listing.
# Complements scripts/open-core/mcp-community-tools-list-smoke.sh (studio) / MCP stranger exclude.
# Docs: docs/strategy/open-core/KERNEL_VS_PACK_INVENTORY.md
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger CLI help smoke
set -euo pipefail

ROOT="${ZQK_COMMUNITY_ROOT:-}"
if [[ -z "$ROOT" ]]; then
  if [[ -x ./bin/zqk ]]; then
    ROOT=$(pwd)
  elif [[ -x "$(git rev-parse --show-toplevel 2>/dev/null)/bin/zqk" ]]; then
    ROOT="$(git rev-parse --show-toplevel 2>/dev/null)"
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

# Pack ontology CLI discovery that must NOT appear on stranger default help.
PACK_DENY_REGEX='(^|[[:space:]])pplan([[:space:]]|$)|get_current_priority_plan|get_priority_plan_items|get_current_backlog_item|get_next_backlog_item|[[:space:]]orchestrate[[:space:]]|mmorch|cap_orchestrat|cap_night_duty|ORCHESTRATE_PLAN'

OUT=$(mktemp)
LIST=$(mktemp)
# Isolated HOME so operator ~/.cursor / mcp pin cannot hijack stranger help.
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger CLI help smoke
HELP_HOME=$(mktemp -d "${TMPDIR:-/tmp}/zqk-help-home.XXXXXX")
cleanup_help_smoke() {
  rm -f "$OUT" "$LIST" >/dev/null 2>&1 || true
  rm -rf "$HELP_HOME" >/dev/null 2>&1 || true
}
trap cleanup_help_smoke EXIT

echo "cli-community-help-smoke: root=$ROOT binary=$ZQK"

export HOME="$HELP_HOME"
export ZQK_HOME="$HELP_HOME"
unset ZQK_PROJECT_ROOT ZQK_CURRENT_ROOT ZQK_ENABLE_STUDIO_PACK_TOOLS ZQK_STUDIO_DOGFOOD || true

(cd "$ROOT" && "$ZQK" --help 2>/dev/null | tee "$OUT") || {
  echo "FAIL: $ZQK --help exited non-zero" >&2
  exit 1
}

for need in object system workflow mcp version; do
  if ! grep -Eq "^[[:space:]]+${need}[[:space:]]" "$OUT"; then
    echo "FAIL: required command missing from help: $need" >&2
    exit 1
  fi
done

HITS=$(grep -E "$PACK_DENY_REGEX" "$OUT" || true)
if [[ -n "$HITS" ]]; then
  echo "FAIL: pack ontology CLI still exposed on community/stranger help:" >&2
  echo "$HITS" >&2
  echo "See KERNEL_VS_PACK_INVENTORY.md + OPEN_CORE_COMMUNITY_COMMAND_ALLOWLIST.md" >&2
  exit 1
fi

(cd "$ROOT" && "$ZQK" 2>/dev/null | tee "$LIST") || true
if [[ -s "$LIST" ]]; then
  LIST_HITS=$(grep -E "$PACK_DENY_REGEX" "$LIST" || true)
  if [[ -n "$LIST_HITS" ]]; then
    echo "FAIL: pack ontology CLI still exposed on community/stranger command listing:" >&2
    echo "$LIST_HITS" >&2
    exit 1
  fi
fi

echo "PASS: community CLI help has required commands and no pack ontology discovery"
