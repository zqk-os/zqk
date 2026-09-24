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
pkg/billing
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
      ! -name 'generate-openvex.sh' \
      ! -name 'check-hardcoded-paths-and-perms-repo.sh' \
      ! -name 'check-cli-name-literals-repo.sh' \
      ! -name 'scan-secrets.sh' \
      ! -path '*/open-core/*' \
      ! -path '*/starter_kernel_graph/*' \
      ! -path '*/onboarding_roadmap/*' \
      ! -path '*/default_agent_skills/*' \
      ! -path '*/default_policies/*' \
      ! -path '*/demos/*' \
      ! -name 'build-bootstrap-archive.sh' \
      2>/dev/null || true)
    if [ -n "$extras" ]; then
      echo "MUST_NOT extra scripts (G11):"
      echo "$extras"
      FAIL=1
    fi
    studio_diffs=$(find "$ROOT/scripts" -name 'community-to-studio-*' 2>/dev/null || true)
    if [ -n "$studio_diffs" ]; then
      echo "MUST_NOT leftover studio patch dump:"
      echo "$studio_diffs"
      FAIL=1
    fi
  fi
  # Archived docs must not be present in candidate docs.
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

# Studio script bleedthrough prohibition: Open-Core must never reference studio-only scripts.
for bad_script in 'install-zqk-stable\.sh' 'recycle-stable-daemons\.sh' 'declare-change-intent\.sh' 'check-change-intent\.sh'; do
  if [ -d "$ROOT/.git" ] && command -v git >/dev/null 2>&1; then
    if git -C "$ROOT" grep -n -E "$bad_script" -- . ':!scripts/open-core/police-community-tree.sh' >/dev/null 2>&1; then
      echo "MUST_NOT studio script reference found ($bad_script):"
      git -C "$ROOT" grep -n -E "$bad_script" -- . ':!scripts/open-core/police-community-tree.sh' || true
      FAIL=1
    fi
  fi
done

if [ "$FAIL" -ne 0 ]; then
  echo "POLICE: FAIL"
  exit 1
fi
echo "POLICE: PASS"
exit 0
