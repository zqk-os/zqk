#!/bin/sh
# Police a community candidate tree using MUST_NOT globs (gitignore-mined + G12/G14).
# Usage: police-community-tree.sh <candidate-root>
set -eu
ROOT=${1:-}
if [ -z "$ROOT" ] || [ ! -d "$ROOT" ]; then
  echo "usage: $0 <candidate-root>" >&2
  exit 2
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)
FAIL=0

echo "Police root: $ROOT"

must_not_paths='
docs/commercial
docs/marketing
docs/launch
marketing-site
.workstream-os
cmd/build
cmd/codegen_runner
cmd/pattern-cli
cmd/utilities
scripts/zqk-internal
scripts/legacy-decommission
'

for p in $must_not_paths; do
  if [ -e "$ROOT/$p" ]; then
    echo "MUST_NOT present: $p"
    FAIL=1
  fi
done

must_not_files='
ack.txt
remaining.txt
issues.json
planned_blis.json
dry_caches.py
refactor.py
fake_editor.sh
cap_background.sh
cap_orchestrator.sh
fix_refs.sh
register_docs.sh
c-corp-filing.png
domains-registered.png
institutionalize_phase4
'

for f in $must_not_files; do
  if [ -e "$ROOT/$f" ]; then
    echo "MUST_NOT file: $f"
    FAIL=1
  fi
done

# Globs at repo root
for g in zqk-demo zqk-ffmpeg-worker zqk-shim mcp-simple; do
  if [ -e "$ROOT/$g" ]; then
    echo "MUST_NOT binary/dir: $g"
    FAIL=1
  fi
done

if command -v find >/dev/null 2>&1; then
  # Living community kernel ships release overlays and first-run seed scripts.
  if [ -d "$ROOT/scripts" ]; then
    extras=$(find "$ROOT/scripts" -type f \( -name '*.sh' -o -name '*.py' \) \
      ! -name 'package-community.sh' \
      ! -name 'install.sh' \
      ! -path '*/open-core/*' \
      ! -path '*/starter_kernel_graph/*' \
      ! -path '*/onboarding_roadmap/*' \
      ! -path '*/default_agent_skills/*' \
      ! -path '*/default_policies/*' \
      ! -name 'build-bootstrap-archive.sh' \
      2>/dev/null || true)
    if [ -n "$extras" ]; then
      echo "MUST_NOT extra scripts (G11):"
      echo "$extras"
      FAIL=1
    fi
  fi
  # Archived docs must not be present in candidate docs (TDE-1789629838711755000-bc3ed5d4)
  if [ -d "$ROOT/docs" ]; then
    archived_docs=$(find "$ROOT/docs" \( -name "archive" -o -name "_archive" -o -path '*/archive/*' -o -path '*/_archive/*' \) 2>/dev/null || true)
    if [ -n "$archived_docs" ]; then
      echo "MUST_NOT archived docs present:"
      echo "$archived_docs"
      FAIL=1
    fi
  fi
fi

if [ -f "$ROOT/.gitignore" ]; then
  for bad in '*-*.txt' '*test*.txt' '*_results.txt'; do
    if grep -Fqx "$bad" "$ROOT/.gitignore" 2>/dev/null || grep -F "$bad" "$ROOT/.gitignore" >/dev/null 2>&1; then
      # only fail on exact line match
      if grep -Fqx "$bad" "$ROOT/.gitignore"; then
        echo "MUST_NOT ignore pattern (too broad): $bad"
        FAIL=1
      fi
    fi
  done
fi

if [ "$FAIL" -ne 0 ]; then
  echo "POLICE: FAIL"
  exit 1
fi
echo "POLICE: PASS"
exit 0
