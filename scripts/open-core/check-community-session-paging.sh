#!/bin/sh
# Fail-closed: stranger session continuity uses community kernel (whats-next / objects),
# not Studio-only virtual-context paging packages.
set -eu
ROOT=${1:-}
if [ -z "$ROOT" ]; then
  ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)
FAIL=0
say() { printf '%s\n' "$*"; }
fail() { say "FAIL: $*"; FAIL=1; }

say "# community session paging (v0.1 kernel-as-SSOT)"
say "root=$ROOT"

ZQK=""
for cand in "$ROOT/bin/zqk-community" "$ROOT/bin/zqk"; do
  [ -x "$cand" ] && ZQK=$cand && break
done
[ -n "$ZQK" ] || { fail "missing bin/zqk under $ROOT"; exit 1; }

# Studio private module must not appear in community main deps
if command -v go >/dev/null 2>&1; then
  PKG=./cmd/zqk
  [ -d "$ROOT/cmd/zqk-community" ] && [ ! -d "$ROOT/cmd/zqk" ] && PKG=./cmd/zqk-community
  if [ -d "$ROOT/cmd/zqk" ] || [ -d "$ROOT/cmd/zqk-community" ]; then
    hits=$(cd "$ROOT" && go list -deps "$PKG" 2>/dev/null | grep -E 'github\.com/zqk-os/zqke(/|$)|virtualcontext|studio/paging' || true)
    if [ -n "$hits" ]; then
      fail "community main depends on studio paging / private zqke module:"
      say "$hits"
    else
      say "OK: community main deps free of private zqke / studio paging modules"
    fi
  fi
fi

DOC="$ROOT/docs/strategy/open-core/CONTEXT_PAGING_DESIGN.md"
[ -f "$DOC" ] || DOC="$ROOT/docs/architecture/CONTEXT_PAGING_DESIGN.md"
[ -f "$DOC" ] || fail "missing CONTEXT_PAGING_DESIGN.md (honest v0.1 paging position)"
if [ -f "$DOC" ]; then
  grep -F 'whats-next' "$DOC" >/dev/null 2>&1 || fail "design note missing whats-next restart path"
  say "OK: design note documents kernel whats-next continuity"
fi

TMP=$(mktemp -d /tmp/zqk-session-paging.XXXXXX)
trap 'rm -rf "$TMP"' EXIT
(
  cd "$TMP"
  if ! "$ZQK" init --format json >/tmp/zqk-paging-init.out 2>&1; then
    "$ZQK" init >/tmp/zqk-paging-init.out 2>&1 || true
  fi
  out=$("$ZQK" workflow whats-next --skip-measure --format json 2>/tmp/zqk-paging-wn.err || true)
  if echo "$out" | grep -Eq 'zqk_whats_next_v1|"schema"'; then
    say "OK: whats-next returns structured kernel context"
  else
    fail "whats-next did not return structured context in greenfield"
    say "stderr: $(head -c 400 /tmp/zqk-paging-wn.err 2>/dev/null || true)"
  fi
)

if [ "$FAIL" -ne 0 ]; then
  say "COMMUNITY SESSION PAGING: FAIL"
  exit 1
fi
say "COMMUNITY SESSION PAGING: PASS"
exit 0
