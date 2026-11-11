#!/usr/bin/env bash
# Community LLM/provider discovery smoke (stranger profile).
# Fail-closed if discovery sources are missing, provider listing fails, or Studio
# pack ontology leaks into the community discovery path.
#
# Complements:
#   scripts/open-core/cli-community-help-smoke.sh
#   scripts/open-core/mcp-community-tools-list-smoke.sh
#
# Usage:
#   ZQK_COMMUNITY_ROOT=/path/to/zqk-public-candidate \
#     ./scripts/open-core/llm-community-discovery-smoke.sh
#
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger LLM-discovery residual
set -euo pipefail

ROOT="${ZQK_COMMUNITY_ROOT:-}"
if [[ -z "$ROOT" ]]; then
  if [[ -f ./pkg/llm/discovery.go && -x ./bin/zqk ]]; then
    ROOT=$(pwd)
  elif [[ -f "$(git rev-parse --show-toplevel 2>/dev/null)/pkg/llm/discovery.go" ]]; then
    ROOT="$(git rev-parse --show-toplevel 2>/dev/null)"
  else
    echo "FAIL: set ZQK_COMMUNITY_ROOT to a community checkout with pkg/llm/discovery.go" >&2
    exit 2
  fi
fi
ROOT="$(CDPATH= cd -- "$ROOT" && pwd)"

say() { printf '%s\n' "$*"; }
fail() { say "FAIL: $*" >&2; exit 1; }

PACK_DENY_REGEX='zqk_get_current_priority_plan|zqk_get_priority_plan_items|zqk_get_current_backlog_item|zqk_get_next_backlog_item|get_current_priority_plan|ORCHESTRATE_PLAN|cap_orchestrat|mmorch|zqke-pack-composition'

say "llm-community-discovery-smoke: community_root=$ROOT"

[[ -f "$ROOT/pkg/llm/discovery.go" ]] || fail "missing pkg/llm/discovery.go in community root"
[[ -f "$ROOT/pkg/llm/provider_registry.go" ]] || fail "missing pkg/llm/provider_registry.go in community root"
[[ -f "$ROOT/pkg/llm/provider.go" ]] || fail "missing pkg/llm/provider.go in community root"

if [[ "$ROOT" == *zqke-pack-composition* ]]; then
  fail "community root resolves to studio checkout; set ZQK_COMMUNITY_ROOT to public-candidate/core"
fi

# Enumerate provider engines from community registry (must list providers).
PROVIDER_LIST="$(mktemp "${TMPDIR:-/tmp}/zqk-llm-providers.XXXXXX")"
TEST_OUT="$(mktemp "${TMPDIR:-/tmp}/zqk-llm-discovery.XXXXXX")"
HOME_DIR="$(mktemp -d "${TMPDIR:-/tmp}/zqk-llm-discovery-home.XXXXXX")"
cleanup() {
  rm -f "$PROVIDER_LIST" "$TEST_OUT" >/dev/null 2>&1 || true
  rm -rf "$HOME_DIR" >/dev/null 2>&1 || true
}
trap cleanup EXIT

grep -E 'Engine[A-Za-z0-9]+[[:space:]]+ProviderEngineType[[:space:]]+=[[:space:]]+"[^"]+"' \
  "$ROOT/pkg/llm/provider_registry.go" | tee "$PROVIDER_LIST"
[[ -s "$PROVIDER_LIST" ]] || fail "no ProviderEngineType constants listed in provider_registry.go"
for eng in ollama vllm llama-server custom-local; do
  grep -Fq "\"$eng\"" "$PROVIDER_LIST" || fail "provider engine '$eng' not listed"
done
say "providers_listed=$(wc -l < "$PROVIDER_LIST" | tr -d ' ')"

# Federation suite exercises Register/Get/List/BestAvailable + discovery fallback.
(
  cd "$ROOT"
  unset ZQK_PROJECT_ROOT ZQK_CURRENT_ROOT ZQK_ENABLE_STUDIO_PACK_TOOLS ZQK_STUDIO_DOGFOOD 2>/dev/null || true
  # Keep host Go module cache; only clear Studio ZQK pins for stranger path.
  export GOMODCACHE="${GOMODCACHE:-$(go env GOMODCACHE)}"
  go test ./pkg/community -run 'TestModelFederation_' -count=1 -timeout 60s -v
) | tee "$TEST_OUT"

grep -Eq 'PASS: TestModelFederation_StaticFloorInvariant' "$TEST_OUT" \
  || fail "StaticFloorInvariant did not PASS (provider registry list/get)"
grep -Eq 'PASS: TestModelFederation_OperationalProof' "$TEST_OUT" \
  || fail "OperationalProof did not PASS (discovered models / fallback)"
grep -Eq 'ok[[:space:]]+github.com/zqk-os/zqk/pkg/community' "$TEST_OUT" \
  || fail "pkg/community ModelFederation suite did not report ok"

HITS=$(grep -E "$PACK_DENY_REGEX" "$TEST_OUT" "$PROVIDER_LIST" || true)
if [[ -n "$HITS" ]]; then
  say "FAIL: Studio/pack ontology leaked into LLM discovery output:" >&2
  say "$HITS" >&2
  exit 1
fi

if [[ -x "$ROOT/bin/zqk" ]]; then
  HELP_OUT="$(mktemp "${TMPDIR:-/tmp}/zqk-llm-discovery-help.XXXXXX")"
  HELP_HOME="$(mktemp -d "${TMPDIR:-/tmp}/zqk-llm-discovery-help-home.XXXXXX")"
  (
    export HOME="$HELP_HOME"
    export ZQK_HOME="$HELP_HOME"
    unset ZQK_PROJECT_ROOT ZQK_CURRENT_ROOT ZQK_ENABLE_STUDIO_PACK_TOOLS ZQK_STUDIO_DOGFOOD 2>/dev/null || true
    cd "$ROOT"
    ./bin/zqk --help 2>/dev/null || true
  ) >"$HELP_OUT" || true
  HELP_HITS=$(grep -E "$PACK_DENY_REGEX" "$HELP_OUT" || true)
  rm -f "$HELP_OUT" >/dev/null 2>&1 || true
  rm -rf "$HELP_HOME" >/dev/null 2>&1 || true
  if [[ -n "$HELP_HITS" ]]; then
    say "FAIL: pack ontology on community --help during LLM discovery smoke:" >&2
    say "$HELP_HITS" >&2
    exit 1
  fi
fi

say "PASS: community LLM discovery lists providers; pack-ontology-free; no Studio SSOT"
