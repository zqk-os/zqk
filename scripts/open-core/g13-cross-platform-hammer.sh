#!/bin/sh
# G13 hammer (honest): native community build required; windows/amd64 cross-compile probed.
# Android/iOS deferred. Exit 0 only when native build + windows cross-compile both succeed,
# unless ZQK_G13_NATIVE_ONLY=1 (records windows failure as residual — not a full G13 pass).
set -eu
ROOT=${1:-}
if [ -z "$ROOT" ]; then
  ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)
say() { printf '%s\n' "$*"; }
FAIL=0
WIN_FAIL=0
fail() { say "FAIL: $*"; FAIL=1; }

say "# G13 cross-platform hammer"
say "root=$ROOT"
cd "$ROOT"

PKG=./cmd/zqk
[ -d cmd/zqk-community ] && [ ! -f cmd/zqk/main.go ] && PKG=./cmd/zqk-community

say "== native build =="
if go build -o /tmp/zqk-g13-native "$PKG" 2>/tmp/g13-native.err; then
  say "OK: native build ($PKG)"
else
  fail "native build failed: $(head -c 400 /tmp/g13-native.err)"
fi

say "== windows/amd64 cross-compile =="
if GOOS=windows GOARCH=amd64 go build -o /tmp/zqk-windows-amd64.exe "$PKG" 2>/tmp/g13-win.err; then
  say "OK: windows/amd64 build"
else
  WIN_FAIL=1
  say "FAIL: windows/amd64 build (unix syscall coupling):"
  head -c 500 /tmp/g13-win.err || true
  say ""
fi

if command -v docker >/dev/null 2>&1 && [ -f Dockerfile.community ]; then
  say "== docker linux probe (best-effort) =="
  if docker build -f Dockerfile.community -t zqk-g13-linux:local . >/tmp/g13-docker-build.log 2>&1; then
    say "OK: docker build Dockerfile.community"
  else
    say "WARN: docker build failed — see /tmp/g13-docker-build.log"
  fi
else
  say "WARN: docker unavailable or no Dockerfile.community"
fi

say "NOTE: Android/iOS DEFERRED per OPEN_CORE_PUBLIC_RELEASE_HYGIENE G13"

if [ "$FAIL" -ne 0 ]; then
  say "G13 HAMMER: FAIL"
  exit 1
fi
if [ "$WIN_FAIL" -ne 0 ]; then
  if [ "${ZQK_G13_NATIVE_ONLY:-}" = "1" ]; then
    say "G13 HAMMER: PARTIAL (native OK; windows FAIL — residual unix syscall portability)"
    exit 0
  fi
  say "G13 HAMMER: FAIL (windows cross-compile required for full G13)"
  exit 1
fi
say "G13 HAMMER: PASS"
exit 0
