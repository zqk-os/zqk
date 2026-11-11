#!/bin/sh
# G15: product Go must not embed live studio kernel IDs (long form).
# Scope: pkg/** and cmd/zqk*/** non-test *.go (product contracts).
# Pattern matches sync-public-candidate.sh scrub (incl. POL-*).
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE G15
set -eu
ROOT=${1:-}
if [ -z "$ROOT" ]; then
  ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)
say() { printf '%s\n' "$*"; }

say "# G15 studio-ID scrub (non-test product Go)"
say "root=$ROOT"

PATHS=""
for p in pkg cmd/zqk cmd/zqk-community; do
  [ -d "$ROOT/$p" ] && PATHS="$PATHS $ROOT/$p"
done
[ -n "$PATHS" ] || { say "FAIL: no product Go trees"; exit 1; }

# Long-form studio CAS IDs only (not symbolic POL-WORKFLOW-VDS product names).
ID_PAT='(BLI|REQ|DEC|PRI|ATK|CRIT|TEST|GOAL|CVS|POL)-[0-9]{13,}-[a-f0-9]{6,}'

if command -v rg >/dev/null 2>&1; then
  HITS=$(rg -n --glob '*.go' --glob '!*_test.go' -e "$ID_PAT" $PATHS 2>/dev/null || true)
else
  HITS=$(git -C "$ROOT" grep -n -E "$ID_PAT" -- 'pkg/**.go' 'cmd/zqk/**.go' 'cmd/zqk-community/**.go' ':!*_test.go' 2>/dev/null || true)
fi

if [ -n "$HITS" ]; then
  say "FAIL: studio kernel IDs present in non-test product Go:"
  say "$HITS" | head -40
  count=$(printf '%s\n' "$HITS" | wc -l | tr -d ' ')
  say "(showing up to 40 of $count hits)"
  exit 1
fi
say "G15 STUDIO-ID SCRUB: PASS"
exit 0
