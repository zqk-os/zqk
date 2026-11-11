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
PROJ="$TMP/project"
cleanup() {
  # Temp-epoch ambient must not leak after stranger paging probe.
  if [ -x "${ZQK:-}" ] && [ -d "${PROJ:-}" ]; then
    (
      cd "$PROJ"
      unset ZQK_CURRENT_ROOT 2>/dev/null || true
      export ZQK_PROJECT_ROOT="$PROJ"
      "$ZQK" ambient stop >/dev/null 2>&1 || true
    )
  fi
  pkill -f "$TMP" >/dev/null 2>&1 || true
  rm -rf "$TMP" >/dev/null 2>&1 || true
}
trap cleanup EXIT
mkdir -p "$PROJ" "$TMP/home"
(
  cd "$PROJ"
  # Clear studio root bleed-through; isolate HOME so operator MCP/pin paths cannot poison pack-deny.
  unset ZQK_PROJECT_ROOT ZQK_CURRENT_ROOT ZQK_ENABLE_STUDIO_PACK_TOOLS ZQK_STUDIO_DOGFOOD 2>/dev/null || true
  export HOME="$TMP/home"
  export ZQK_HOME="$TMP/home"
  if ! run_bounded 45 "$ZQK" init --format json >"$TMP/init.out" 2>&1; then
    run_bounded 45 "$ZQK" init >"$TMP/init.out" 2>&1 || true
  fi
  out=$(run_bounded 45 "$ZQK" workflow whats-next --skip-measure --format json 2>"$TMP/wn.err" || true)
  printf '%s\n' "$out" >"$TMP/whats-next.json"
  if echo "$out" | grep -Eq 'zqk_whats_next_v1|"schema"'; then
    say "OK: whats-next returns structured kernel context"
  else
    fail "whats-next did not return structured context in greenfield"
    say "stderr: $(head -c 400 "$TMP/wn.err" 2>/dev/null || true)"
  fi
)

# Pack-ontology deny on the stranger whats-next payload (session continuity must stay pack-free).
if [ -s "$TMP/whats-next.json" ] && [ "$FAIL" -eq 0 ]; then
  if ! python3 - "$TMP/whats-next.json" <<'PY'
import json, re, sys
path = sys.argv[1]
raw = open(path, encoding="utf-8").read()
try:
    d = json.loads(raw)
except Exception as e:
    print(f"FAIL: whats-next JSON parse: {e}", file=sys.stderr)
    sys.exit(1)
scrub = dict(d)
ka = scrub.get("kernel_ambience")
if isinstance(ka, dict):
    ka = dict(ka)
    ka.pop("node_sovereignty", None)
    scrub["kernel_ambience"] = ka
blob = json.dumps(scrub, sort_keys=True)
deny = re.compile(
    r"(?i)"
    r"(PER-ORCH-|PER-STUDIO-|ASK-STUDIO-|ASK-CAP-|"
    r"mmorch|ORCHESTRATE_PLAN|"
    r"get_current_priority_plan|get_priority_plan_items|"
    r"get_current_backlog_item|get_next_backlog_item|"
    r"cap_orchestrat|cap_night_duty|cap_background|"
    r"paste_applescript|print_cursor_paste|print_ide_paste|zqk_cursor|"
    r"align-cache|align_cache|"
    r"zqke-pack-composition)"
)
hit = deny.search(blob)
if hit:
    print(f"FAIL: pack ontology leakage in session-paging whats-next near {hit.group(0)!r}", file=sys.stderr)
    sys.exit(1)
sys.exit(0)
PY
  then
    fail "session-paging stranger whats-next pack-ontology deny failed"
  else
    say "OK: stranger whats-next pack-free under isolated HOME"
  fi
fi

if [ "$FAIL" -ne 0 ]; then
  say "COMMUNITY SESSION PAGING: FAIL"
  exit 1
fi
say "COMMUNITY SESSION PAGING: PASS"
exit 0
