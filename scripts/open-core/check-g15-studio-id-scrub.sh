#!/bin/sh
# G15: non-test product Go must not embed live studio kernel IDs (long form).
# Aligns with sync-public-candidate.sh scrub pattern. Tests may still cite CRIT/BLI in comments.
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

HITS=$(rg -n --glob '*.go' --glob '!*_test.go' \
  -e '(BLI|REQ|DEC|PRI|ATK|CRIT|TEST|GOAL|CVS|POL)-[0-9]{13,}-[a-f0-9]{6,}' \
  $PATHS 2>/dev/null || true)

if [ -n "$HITS" ]; then
  say "FAIL: studio kernel IDs present in non-test product Go:"
  say "$HITS" | head -40
  exit 1
fi
say "G15 STUDIO-ID SCRUB: PASS"
exit 0
