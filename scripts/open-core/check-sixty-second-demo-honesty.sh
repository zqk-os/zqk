#!/bin/sh
# Fail-closed: stranger 60s capability demo must stay community-honest (no Studio-only required path).
set -eu
ROOT=${1:-}
if [ -z "$ROOT" ]; then
  ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)
DOC="$ROOT/docs/demos/SIXTY_SECOND_CAPABILITY.md"
FAIL=0
say() { printf '%s\n' "$*"; }
fail() { say "FAIL: $*"; FAIL=1; }

say "# sixty-second demo honesty"
say "root=$ROOT doc=$DOC"
[ -f "$DOC" ] || { fail "missing $DOC"; exit 1; }

if grep -E 'docs/launch/|\.\./launch/' "$DOC" >/dev/null 2>&1; then
  fail "demo links docs/launch (tree police / Studio path)"
fi
# Required script block must not demand CAP/MMORCH pack surfaces
# Extract fenced script after "## 60-second script"
awk '/^## 60-second script/,/^## /{print}' "$DOC" | grep -E 'mmorch|ORCHESTRATE_PLAN|get_current_priority_plan|zqk do[[:space:]]' >/dev/null 2>&1 \
  && fail "required 60s script references Studio pack/CAP surfaces" || say "OK: required script free of CAP/MMORCH pack commands"

# Must document unauth TTFA path
grep -F 'ttfa-unauth-smoke.sh' "$DOC" >/dev/null 2>&1 || fail "missing timed stranger ttfa-unauth-smoke.sh pointer"
grep -F 'No `GITHUB_TOKEN`' "$DOC" >/dev/null 2>&1 || fail "missing no-token stranger precondition"

if [ "$FAIL" -ne 0 ]; then
  say "SIXTY_SECOND DEMO HONESTY: FAIL"
  exit 1
fi
say "SIXTY_SECOND DEMO HONESTY: PASS"
exit 0
