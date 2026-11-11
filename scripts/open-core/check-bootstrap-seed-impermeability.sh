#!/bin/sh
# Fail-closed CI: community bootstrap archive/manifest must not list CAP/MMORCH/
# orchestrate/TPM operator ontology. Complements pkg/bootstrap denylist extract
# filter. TRACK: BLI-1794356846362471000-b781465c
#
# Usage: check-bootstrap-seed-impermeability.sh [repo-root]
set -eu
ROOT=${1:-.}
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)
MANIFEST="$ROOT/pkg/bootstrap/archive/manifest.txt"
FAIL=0

echo "Bootstrap seed impermeability check: $ROOT"

if [ ! -f "$MANIFEST" ]; then
  echo "FAIL: missing embedded bootstrap manifest: $MANIFEST"
  echo "hint: make bootstrap-archive"
  exit 1
fi

# Keep in sync with pkg/bootstrap/community_denylist.go
FRAGMENTS='
print_cursor_paste_applescript
record_cvs_orchestrate_run
paste_cursor
zqk_cursor
cap_night_duty
cap_orchestrator
cap_background
mmorch
orchestrate_command
orchestrate_batch
tpm_orchestration
/agent/orchestrate
'

hits=""
for frag in $FRAGMENTS; do
  matched=$(grep -i -F -- "$frag" "$MANIFEST" || true)
  if [ -n "$matched" ]; then
    echo "FAIL: denylist fragment '$frag' present in bootstrap manifest:"
    echo "$matched" | sed 's/^/  /'
    FAIL=1
    hits="$hits $frag"
  fi
done

# Unit gate: denylist + community extract behavior
if command -v go >/dev/null 2>&1; then
  echo "Running go test ./pkg/bootstrap -run 'Denylist|CommunityBootstrap' ..."
  if ! (cd "$ROOT" && go test ./pkg/bootstrap -timeout 60s -count=1 -run 'Denylist|CommunityBootstrap|ShouldExcludeCommunity'); then
    echo "FAIL: pkg/bootstrap denylist tests"
    FAIL=1
  fi
else
  echo "WARN: go not on PATH; skipped package denylist tests"
fi

if [ "$FAIL" -ne 0 ]; then
  echo "FAIL: bootstrap seed impermeability (hits:$hits)"
  exit 1
fi
echo "PASS: bootstrap seed impermeability (manifest clean + denylist tests)"
exit 0
