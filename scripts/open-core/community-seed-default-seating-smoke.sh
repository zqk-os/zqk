#!/usr/bin/env bash
# Community default seating seed smoke (stranger profile).
# Fail-closed if scripts/default_personas or scripts/default_agent_skills
# leak Studio pack ontology (CAP/MMORCH/orchestrate/operator paste), or if
# required PER-DEFAULT-* seats are missing.
# Complements check-bootstrap-seed-impermeability.sh (archive denylist).
# Docs: docs/strategy/open-core/COMMUNITY_OBJECT_CLASSIFICATION.md
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger default seating seed
set -euo pipefail

ROOT="${ZQK_COMMUNITY_ROOT:-}"
if [[ -z "$ROOT" ]]; then
  if [[ -x ./bin/zqk ]] && [[ -d ./scripts/default_personas ]]; then
    ROOT=$(pwd)
  elif [[ -x "$(git rev-parse --show-toplevel 2>/dev/null)/bin/zqk" ]]; then
    ROOT="$(git rev-parse --show-toplevel 2>/dev/null)"
  else
    echo "FAIL: set ZQK_COMMUNITY_ROOT to a community checkout with bin/zqk" >&2
    exit 2
  fi
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)

ZQK=""
for cand in "$ROOT/bin/zqk" "$ROOT/bin/zqk-community"; do
  if [[ -x "$cand" ]]; then
    ZQK=$cand
    break
  fi
done
if [[ -z "$ZQK" ]]; then
  echo "FAIL: missing executable bin/zqk under $ROOT" >&2
  exit 2
fi

PERSONAS_DIR="$ROOT/scripts/default_personas"
SKILLS_DIR="$ROOT/scripts/default_agent_skills"
if [[ ! -d "$PERSONAS_DIR" ]]; then
  echo "FAIL: missing $PERSONAS_DIR" >&2
  exit 1
fi
if [[ ! -d "$SKILLS_DIR" ]]; then
  echo "FAIL: missing $SKILLS_DIR" >&2
  exit 1
fi

# Pack ontology / Studio operator seating that must NOT appear in stranger seed templates.
PACK_DENY_REGEX='PER-ORCH-|PER-STUDIO-|ASK-STUDIO-|ASK-CAP-|mmorch|ORCHESTRATE_PLAN|get_current_priority_plan|get_priority_plan_items|get_current_backlog_item|get_next_backlog_item|cap_orchestrat|cap_night_duty|paste_applescript|print_cursor_paste|print_ide_paste|zqk_cursor|ACC-TEST-HARNESS|cap_background'

# Allowed community / kernel-generic seating id prefixes (classification + system library).
ALLOW_ID_REGEX='^(PER-DEFAULT-|PER-COMMUNITY-|PER-SYSTEM-|ASK-DEFAULT-|ASK-COMMUNITY-|ASK-SYSTEM-)'

echo "community-seed-default-seating-smoke: root=$ROOT binary=$ZQK"

unset ZQK_ENABLE_STUDIO_PACK_TOOLS ZQK_STUDIO_DOGFOOD || true

# Required stranger seats (embedded fallback + on-disk templates).
need_ids=(PER-DEFAULT-OPERATOR PER-DEFAULT-AGENT ASK-DEFAULT-FEED-CORRESPONDENCE)
for need in "${need_ids[@]}"; do
  if ! grep -rqF --include='*.yaml' --include='*.yml' "id: $need" "$PERSONAS_DIR" "$SKILLS_DIR"; then
    echo "FAIL: required seating id missing from default templates: $need" >&2
    exit 1
  fi
done
echo "OK: required PER-DEFAULT / ASK-DEFAULT seating ids present"

# Fail-closed: every template id must be community/kernel-generic prefix.
bad_ids=$(grep -rhoE --include='*.yaml' --include='*.yml' '^id:[[:space:]]*[^[:space:]]+' "$PERSONAS_DIR" "$SKILLS_DIR" \
  | sed -E 's/^id:[[:space:]]*//' \
  | while read -r id; do
      [[ -z "$id" ]] && continue
      if ! [[ "$id" =~ $ALLOW_ID_REGEX ]]; then
        printf '%s\n' "$id"
      fi
    done)
if [[ -n "$bad_ids" ]]; then
  echo "FAIL: seating template ids outside community/kernel-generic allowlist:" >&2
  echo "$bad_ids" >&2
  exit 1
fi
echo "OK: seating template ids match community/kernel-generic allowlist"

# Content scan for pack ontology markers.
scan_tmp=$(mktemp)
trap 'rm -f "$scan_tmp"' EXIT
set +e
grep -r -n -i -E --include='*.yaml' --include='*.yml' "$PACK_DENY_REGEX" "$PERSONAS_DIR" "$SKILLS_DIR" >"$scan_tmp" 2>&1
scan_rc=$?
set -e
if [[ $scan_rc -gt 1 ]]; then
  echo "FAIL: pack ontology seating scan errored:" >&2
  cat "$scan_tmp" >&2
  exit 1
fi
if [[ $scan_rc -eq 0 ]]; then
  echo "FAIL: pack ontology markers in default seating templates:" >&2
  cat "$scan_tmp" >&2
  echo "See COMMUNITY_OBJECT_CLASSIFICATION.md + KERNEL_VS_PACK_INVENTORY.md" >&2
  exit 1
fi
echo "OK: seating templates free of pack ontology deny markers"

# Binary must expose the repair command strangers are told to run.
if ! "$ZQK" system seed-default-agent-seating --help >/dev/null 2>&1; then
  echo "FAIL: $ZQK system seed-default-agent-seating --help failed" >&2
  exit 1
fi
echo "OK: system seed-default-agent-seating present"

# Archive impermeability when candidate embeds bootstrap archive (PC layout).
IMPERMEABILITY=""
if [[ -f "$ROOT/scripts/open-core/check-bootstrap-seed-impermeability.sh" ]]; then
  IMPERMEABILITY="$ROOT/scripts/open-core/check-bootstrap-seed-impermeability.sh"
elif [[ -f "$(dirname "$0")/check-bootstrap-seed-impermeability.sh" ]]; then
  IMPERMEABILITY="$(CDPATH= cd -- "$(dirname "$0")" && pwd)/check-bootstrap-seed-impermeability.sh"
fi
if [[ -n "$IMPERMEABILITY" ]] && {
  [[ -f "$ROOT/pkg/bootstrap/archive/manifest.txt" ]] || [[ -f "$ROOT/pkg/bootstrap/archive/bootstrap.tar.gz" ]]
}; then
  if ! sh "$IMPERMEABILITY" "$ROOT"; then
    echo "FAIL: bootstrap seed impermeability failed (nested seating gate)" >&2
    exit 1
  fi
  echo "OK: nested bootstrap seed impermeability PASS"
fi

# Working-tree seating must not reintroduce scrubbed denylist filenames into the
# bootstrap archive listing (tpm_orchestration scrubbed; community skill may
# remain on disk outside the archive).
if [[ -f "$ROOT/pkg/bootstrap/archive/manifest.txt" ]]; then
  if grep -q -F 'tpm_orchestration' "$ROOT/pkg/bootstrap/archive/manifest.txt"; then
    echo "FAIL: bootstrap manifest still lists tpm_orchestration (pack/operator seed leak)" >&2
    exit 1
  fi
  echo "OK: bootstrap manifest has no tpm_orchestration seating path"
fi

echo "PASS: community default seating seed stranger-clean (no pack ontology leakage)"
