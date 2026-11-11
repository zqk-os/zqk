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
internal/billing
pkg/economy
internal/economy
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
      ! -name 'install_test.sh' \
      ! -name 'generate-openvex.sh' \
      ! -name 'check-hardcoded-paths-and-perms-repo.sh' \
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
  fi
  # Archived docs must not be present in candidate docs
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

# Seed impermeability when candidate embeds pkg/bootstrap/archive (PC layout).
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — bootstrap seed impermeability
if [ -f "$ROOT/scripts/open-core/check-bootstrap-seed-impermeability.sh" ] && \
   { [ -f "$ROOT/pkg/bootstrap/archive/manifest.txt" ] || [ -f "$ROOT/pkg/bootstrap/archive/bootstrap.tar.gz" ]; }; then
  if ! sh "$ROOT/scripts/open-core/check-bootstrap-seed-impermeability.sh" "$ROOT"; then
    echo "MUST_NOT bootstrap seed impermeability failed"
    FAIL=1
  fi
fi

# Default seating seed stranger-clean (no pack ontology leakage in personas/skills).
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger default seating seed
SEATING_SMOKE=""
if [ -f "$ROOT/scripts/open-core/community-seed-default-seating-smoke.sh" ]; then
  SEATING_SMOKE="$ROOT/scripts/open-core/community-seed-default-seating-smoke.sh"
elif [ -f "$(dirname "$0")/community-seed-default-seating-smoke.sh" ]; then
  SEATING_SMOKE="$(CDPATH= cd -- "$(dirname "$0")" && pwd)/community-seed-default-seating-smoke.sh"
fi
if [ -n "$SEATING_SMOKE" ] && [ -d "$ROOT/scripts/default_personas" ]; then
  if ! ZQK_COMMUNITY_ROOT="$ROOT" bash "$SEATING_SMOKE"; then
    echo "MUST_NOT community default seating seed smoke failed"
    FAIL=1
  fi
fi

# Virtual-context paging residual: stranger sessions keep local context without Studio paging deps.
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — community session paging
if [ -f "$ROOT/scripts/open-core/check-community-session-paging.sh" ] && \
   { [ -d "$ROOT/pkg/sessionpaging" ] || [ -f "$ROOT/docs/strategy/open-core/CONTEXT_PAGING_DESIGN.md" ]; }; then
  if ! sh "$ROOT/scripts/open-core/check-community-session-paging.sh" "$ROOT"; then
    echo "MUST_NOT community session paging residual failed"
    FAIL=1
  fi
fi

# Bounded stranger whats-next: community starter plan only, no pack ontology leakage.
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger whats-next residual
WN_BOUNDED=""
if [ -f "$ROOT/scripts/open-core/check-community-whats-next-bounded.sh" ]; then
  WN_BOUNDED="$ROOT/scripts/open-core/check-community-whats-next-bounded.sh"
elif [ -f "$(dirname "$0")/check-community-whats-next-bounded.sh" ]; then
  WN_BOUNDED="$(CDPATH= cd -- "$(dirname "$0")" && pwd)/check-community-whats-next-bounded.sh"
fi
if [ -n "$WN_BOUNDED" ]; then
  if ! ZQK_BIN="${ZQK_BIN:-$ROOT/bin/zqk}" sh "$WN_BOUNDED" "$ROOT"; then
    echo "MUST_NOT community whats-next bounded residual failed"
    FAIL=1
  fi
fi

# Bounded stranger feed doctor/status: HOME-isolated greenfield, no pack ontology leakage.
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger doctor residual
DOCTOR_BOUNDED=""
if [ -f "$ROOT/scripts/open-core/check-community-doctor-bounded.sh" ]; then
  DOCTOR_BOUNDED="$ROOT/scripts/open-core/check-community-doctor-bounded.sh"
elif [ -f "$(dirname "$0")/check-community-doctor-bounded.sh" ]; then
  DOCTOR_BOUNDED="$(CDPATH= cd -- "$(dirname "$0")" && pwd)/check-community-doctor-bounded.sh"
fi
if [ -n "$DOCTOR_BOUNDED" ]; then
  if ! ZQK_BIN="${ZQK_BIN:-$ROOT/bin/zqk}" sh "$DOCTOR_BOUNDED" "$ROOT"; then
    echo "MUST_NOT community doctor bounded residual failed"
    FAIL=1
  fi
fi

# G15: fail-closed studio kernel-ID scrub on product Go (non-test).
# Prefer candidate-local script; fall back to studio police host copy.
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE G15
G15_SCRIPT=""
if [ -f "$ROOT/scripts/open-core/check-g15-studio-id-scrub.sh" ]; then
  G15_SCRIPT="$ROOT/scripts/open-core/check-g15-studio-id-scrub.sh"
elif [ -f "$(dirname "$0")/check-g15-studio-id-scrub.sh" ]; then
  G15_SCRIPT="$(CDPATH= cd -- "$(dirname "$0")" && pwd)/check-g15-studio-id-scrub.sh"
fi
if [ -n "$G15_SCRIPT" ]; then
  if ! sh "$G15_SCRIPT" "$ROOT"; then
    echo "MUST_NOT G15 studio-ID scrub failed"
    FAIL=1
  fi
else
  echo "MUST_NOT missing check-g15-studio-id-scrub.sh (G15 fail-closed)"
  FAIL=1
fi

# EE packaging boundary: community/public tree + package-community must not ship EE.
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — EE packaging boundary (PRI lead)
EE_SCRIPT=""
if [ -f "$ROOT/scripts/open-core/check-ee-packaging-boundary.sh" ]; then
  EE_SCRIPT="$ROOT/scripts/open-core/check-ee-packaging-boundary.sh"
elif [ -f "$(dirname "$0")/check-ee-packaging-boundary.sh" ]; then
  EE_SCRIPT="$(CDPATH= cd -- "$(dirname "$0")" && pwd)/check-ee-packaging-boundary.sh"
fi
if [ -n "$EE_SCRIPT" ]; then
  if ! sh "$EE_SCRIPT" "$ROOT" community; then
    echo "MUST_NOT EE packaging boundary failed"
    FAIL=1
  fi
else
  echo "MUST_NOT missing check-ee-packaging-boundary.sh (EE packaging fail-closed)"
  FAIL=1
fi

if [ "$FAIL" -ne 0 ]; then
  echo "POLICE: FAIL"
  exit 1
fi
echo "POLICE: PASS"
exit 0
