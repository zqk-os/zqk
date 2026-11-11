#!/bin/sh
# Fail-closed: stranger session continuity uses community kernel (whats-next / objects)
# plus local pkg/sessionpaging — not Studio-only virtual-context paging packages.
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — community session paging
set -eu
ROOT=${1:-}
if [ -z "$ROOT" ]; then
  ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)
FAIL=0
say() { printf '%s\n' "$*"; }
fail() { say "FAIL: $*"; FAIL=1; }

say "# community session paging (v0.1 kernel-as-SSOT + local hot pages)"
say "root=$ROOT"

ZQK=""
for cand in "$ROOT/bin/zqk-community" "$ROOT/bin/zqk"; do
  [ -x "$cand" ] && ZQK=$cand && break
done
[ -n "$ZQK" ] || { fail "missing bin/zqk under $ROOT"; exit 1; }

# Community-local hot page store must exist (no Studio package required).
if [ ! -f "$ROOT/pkg/sessionpaging/paging.go" ]; then
  fail "missing pkg/sessionpaging/paging.go (community-local hot pages)"
else
  say "OK: pkg/sessionpaging present"
fi

# Studio private / paging modules must not appear in go deps (imports), not deny-list string literals.
if command -v go >/dev/null 2>&1; then
  if [ -d "$ROOT/pkg/sessionpaging" ]; then
    hits=$(cd "$ROOT" && go list -deps ./pkg/sessionpaging 2>/dev/null | grep -E 'github\.com/zqk-os/zqke(/|$)|/virtualcontext($|/)|studio/paging' || true)
    if [ -n "$hits" ]; then
      fail "pkg/sessionpaging go deps include studio paging / private zqke module:"
      say "$hits"
    else
      say "OK: pkg/sessionpaging go deps free of private zqke / studio paging modules"
    fi
  fi
  PKG=./cmd/zqk
  [ -d "$ROOT/cmd/zqk-community" ] && [ ! -d "$ROOT/cmd/zqk" ] && PKG=./cmd/zqk-community
  if [ -d "$ROOT/cmd/zqk" ] || [ -d "$ROOT/cmd/zqk-community" ]; then
    hits=$(cd "$ROOT" && go list -deps "$PKG" 2>/dev/null | grep -E 'github\.com/zqk-os/zqke(/|$)|/virtualcontext($|/)|studio/paging' || true)
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
  grep -F 'sessionpaging' "$DOC" >/dev/null 2>&1 || fail "design note missing pkg/sessionpaging local hot pages"
  say "OK: design note documents kernel whats-next + local sessionpaging"
fi

# Hot-page continuity: Put/Get/evict + refuse Studio module paths (no Studio pack/CAP).
if command -v go >/dev/null 2>&1 && [ -d "$ROOT/pkg/sessionpaging" ]; then
  if (
    cd "$ROOT" && go test ./pkg/sessionpaging -timeout 60s -count=1 \
      -run 'TestStore_PutGetEvict|TestRefuseStudioModulePath|TestStore_FailClosedEmptyAndUnsafe|TestStore_PagesUnderStateTree' \
      >/tmp/zqk-session-paging-gotest.out 2>&1
  ); then
    say "OK: pkg/sessionpaging hot-page continuity unit tests"
  else
    fail "pkg/sessionpaging hot-page continuity unit tests failed"
    say "$(tail -c 600 /tmp/zqk-session-paging-gotest.out 2>/dev/null || true)"
  fi
else
  fail "go + pkg/sessionpaging required for hot-page continuity proof"
fi

# Bound greenfield whats-next so the gate cannot hang on slow init trees (macOS: perl alarm).
run_bounded() {
  _secs=$1
  shift
  if command -v perl >/dev/null 2>&1; then
    perl -e "alarm int(shift); exec @ARGV" -- "$_secs" "$@"
  else
    "$@"
  fi
}

TMP=$(mktemp -d /tmp/zqk-session-paging.XXXXXX)
cleanup() { rm -rf "$TMP" >/dev/null 2>&1 || true; }
trap cleanup EXIT
(
  cd "$TMP"
  # Clear studio root bleed-through so stranger continuity is community-local.
  unset ZQK_PROJECT_ROOT ZQK_CURRENT_ROOT 2>/dev/null || true
  if ! run_bounded 45 "$ZQK" init --format json >"$TMP/init.out" 2>&1; then
    run_bounded 45 "$ZQK" init >"$TMP/init.out" 2>&1 || true
  fi
  out=$(run_bounded 45 "$ZQK" workflow whats-next --skip-measure --format json 2>"$TMP/wn.err" || true)
  if echo "$out" | grep -Eq 'zqk_whats_next_v1|"schema"'; then
    say "OK: whats-next returns structured kernel context"
  else
    fail "whats-next did not return structured context in greenfield"
    say "stderr: $(head -c 400 "$TMP/wn.err" 2>/dev/null || true)"
  fi
)

if [ "$FAIL" -ne 0 ]; then
  say "COMMUNITY SESSION PAGING: FAIL"
  exit 1
fi
say "COMMUNITY SESSION PAGING: PASS"
exit 0
