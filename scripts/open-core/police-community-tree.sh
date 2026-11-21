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

POLICE_WORK=$(mktemp -d "${TMPDIR:-/tmp}/zqk-police-tree.XXXXXX")
trap 'rm -rf "$POLICE_WORK" >/dev/null 2>&1 || true' EXIT HUP INT TERM

# Run independent heavy sub-checks concurrently to keep total wall-clock well under
# the Go test 10m timeout without weakening any fail-closed check.
P_JOBS=""
add_job() {
  _idx=$1
  _pid=$2
  _msg=$3
  printf '%s\n' "$_msg" >"$POLICE_WORK/${_idx}.msg"
  P_JOBS="$P_JOBS ${_idx}:${_pid}"
}

# Default seating seed stranger-clean (no pack ontology leakage in personas/skills).
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger default seating seed
SEATING_SMOKE=""
if [ -f "$ROOT/scripts/open-core/community-seed-default-seating-smoke.sh" ]; then
  SEATING_SMOKE="$ROOT/scripts/open-core/community-seed-default-seating-smoke.sh"
elif [ -f "$(dirname "$0")/community-seed-default-seating-smoke.sh" ]; then
  SEATING_SMOKE="$(CDPATH= cd -- "$(dirname "$0")" && pwd)/community-seed-default-seating-smoke.sh"
fi
if [ -n "$SEATING_SMOKE" ] && [ -d "$ROOT/scripts/default_personas" ]; then
  ZQK_COMMUNITY_ROOT="$ROOT" ZQK_BIN="${ZQK_BIN:-$ROOT/bin/zqk}" bash "$SEATING_SMOKE" >"$POLICE_WORK/1.log" 2>&1 &
  add_job 1 "$!" "MUST_NOT community default seating seed smoke failed"
fi

# Virtual-context paging residual: stranger sessions keep local context without Studio paging deps.
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — community session paging
if [ -f "$ROOT/scripts/open-core/check-community-session-paging.sh" ] && \
   { [ -d "$ROOT/pkg/sessionpaging" ] || [ -f "$ROOT/docs/strategy/open-core/CONTEXT_PAGING_DESIGN.md" ]; }; then
  ZQK_BIN="${ZQK_BIN:-$ROOT/bin/zqk}" sh "$ROOT/scripts/open-core/check-community-session-paging.sh" "$ROOT" >"$POLICE_WORK/2.log" 2>&1 &
  add_job 2 "$!" "MUST_NOT community session paging residual failed"
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
  ZQK_BIN="${ZQK_BIN:-$ROOT/bin/zqk}" bash "$WN_BOUNDED" "$ROOT" >"$POLICE_WORK/3.log" 2>&1 &
  add_job 3 "$!" "MUST_NOT community whats-next bounded residual failed"
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
  ZQK_BIN="${ZQK_BIN:-$ROOT/bin/zqk}" bash "$DOCTOR_BOUNDED" "$ROOT" >"$POLICE_WORK/4.log" 2>&1 &
  add_job 4 "$!" "MUST_NOT community doctor bounded residual failed"
fi

# Stranger MCP tools/list: HOME-isolated cell, no pack ontology getters.
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger MCP tools/list residual
MCP_TOOLS=""
if [ -f "$ROOT/scripts/open-core/mcp-community-tools-list-smoke.sh" ]; then
  MCP_TOOLS="$ROOT/scripts/open-core/mcp-community-tools-list-smoke.sh"
elif [ -f "$(dirname "$0")/mcp-community-tools-list-smoke.sh" ]; then
  MCP_TOOLS="$(CDPATH= cd -- "$(dirname "$0")" && pwd)/mcp-community-tools-list-smoke.sh"
fi
if [ -n "$MCP_TOOLS" ]; then
  ZQK_BIN="${ZQK_BIN:-$ROOT/bin/zqk}" bash "$MCP_TOOLS" "$ROOT" >"$POLICE_WORK/5.log" 2>&1 &
  add_job 5 "$!" "MUST_NOT community MCP tools/list residual failed"
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
  sh "$G15_SCRIPT" "$ROOT" >"$POLICE_WORK/6.log" 2>&1 &
  add_job 6 "$!" "MUST_NOT G15 studio-ID scrub failed"
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
  sh "$EE_SCRIPT" "$ROOT" community >"$POLICE_WORK/7.log" 2>&1 &
  add_job 7 "$!" "MUST_NOT EE packaging boundary failed"
else
  echo "MUST_NOT missing check-ee-packaging-boundary.sh (EE packaging fail-closed)"
  FAIL=1
fi

# Payload TRACK/nanos scrub: police must fail-closed when public-release payload is dirty.
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — police/payload residual after launch-gate
PAYLOAD_SCRIPT=""
if [ -f "$ROOT/scripts/open-core/check-public-release-payload.sh" ]; then
  PAYLOAD_SCRIPT="$ROOT/scripts/open-core/check-public-release-payload.sh"
elif [ -f "$(dirname "$0")/check-public-release-payload.sh" ]; then
  PAYLOAD_SCRIPT="$(CDPATH= cd -- "$(dirname "$0")" && pwd)/check-public-release-payload.sh"
fi
if [ "${ZQK_PAYLOAD_POLICE_GUARD:-}" = "payload" ]; then
  # Invoked from check-public-release-payload.sh; that parent already runs the scrub.
  echo "SKIP: payload TRACK/nanos scrub (parent payload gate is running it)"
elif [ -n "$PAYLOAD_SCRIPT" ]; then
  ZQK_PAYLOAD_POLICE_GUARD=police sh "$PAYLOAD_SCRIPT" "$ROOT" >"$POLICE_WORK/8.log" 2>&1 &
  add_job 8 "$!" "MUST_NOT public-release payload TRACK/nanos scrub failed"
else
  echo "MUST_NOT missing check-public-release-payload.sh (payload scrub fail-closed)"
  FAIL=1
fi

for _job in $P_JOBS; do
  _idx=${_job%%:*}
  _pid=${_job#*:}
  set +e
  wait "$_pid"
  _rc=$?
  set -e
  if [ -f "$POLICE_WORK/${_idx}.log" ]; then
    cat "$POLICE_WORK/${_idx}.log"
  fi
  if [ "$_rc" -ne 0 ]; then
    cat "$POLICE_WORK/${_idx}.msg"
    FAIL=1
  fi
done

if [ "$FAIL" -ne 0 ]; then
  echo "POLICE: FAIL"
  exit 1
fi
echo "POLICE: PASS"
exit 0
