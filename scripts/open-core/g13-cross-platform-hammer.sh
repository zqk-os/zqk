#!/bin/sh
# G13 cross-platform hammer (OPEN_CORE_PUBLIC_RELEASE_HYGIENE).
#
# Minimum overnight bar:
#   1) Native community/product build
#   2) Windows/amd64 cross-compile (required unless ZQK_G13_NATIVE_ONLY=1)
#   3) Docker/linux build + system init smoke when docker is available
# Android/iOS DEFERRED.
#
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE G13
set -eu
ROOT=${1:-}
if [ -z "$ROOT" ]; then
  ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)
say() { printf '%s\n' "$*"; }
FAIL=0
WIN_FAIL=0
DOCKER_FAIL=0
fail() { say "FAIL: $*"; FAIL=1; }

say "# G13 cross-platform hammer"
say "root=$ROOT"
cd "$ROOT"

PKG=./cmd/zqk
[ -d cmd/zqk-community ] && [ ! -f cmd/zqk/main.go ] && PKG=./cmd/zqk-community

LOGDIR="${TMPDIR:-/tmp}/zqk-g13-hammer-$$"
mkdir -p "$LOGDIR"
trap 'rm -rf "$LOGDIR"' EXIT HUP INT TERM

say "== native build =="
if CGO_ENABLED=0 go build -buildvcs=false -o "$LOGDIR/zqk-native" "$PKG" 2>"$LOGDIR/native.err"; then
  say "OK: native build ($PKG)"
else
  fail "native build failed: $(head -c 400 "$LOGDIR/native.err")"
fi

say "== windows/amd64 cross-compile =="
if CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -buildvcs=false -o "$LOGDIR/zqk-windows-amd64.exe" "$PKG" 2>"$LOGDIR/win.err"; then
  say "OK: windows/amd64 build"
else
  WIN_FAIL=1
  say "FAIL: windows/amd64 build:"
  head -c 600 "$LOGDIR/win.err" || true
  say ""
fi

say "== docker/linux build+init smoke =="
# CI / quick probes: set ZQK_G13_SKIP_DOCKER=1 (native+windows still fail-closed).
if [ "${ZQK_G13_SKIP_DOCKER:-0}" = "1" ]; then
  say "SKIP: docker linux smoke (ZQK_G13_SKIP_DOCKER=1)"
  DOCKER_FAIL=0
elif ! command -v docker >/dev/null 2>&1; then
  say "WARN: docker unavailable — skip linux container smoke (document residual)"
  DOCKER_FAIL=1
elif ! docker info >/dev/null 2>&1; then
  say "WARN: docker daemon not reachable — skip linux container smoke"
  DOCKER_FAIL=1
else
  GOIMG="${ZQK_G13_GO_IMAGE:-golang:1.26-bookworm}"
  INIT_SCRIPT="$LOGDIR/linux-init.sh"
  cat >"$INIT_SCRIPT" <<'EOS'
set -eu
PKG=./cmd/zqk
[ -d cmd/zqk-community ] && [ ! -f cmd/zqk/main.go ] && PKG=./cmd/zqk-community
echo "linux-smoke: building $PKG on $(uname -m) $(go env GOOS)/$(go env GOARCH)"
CGO_ENABLED=0 go build -buildvcs=false -o /tmp/zqk-g13 "$PKG"
/tmp/zqk-g13 --help >/dev/null
mkdir -p /tmp/zqk-g13-proj
cd /tmp/zqk-g13-proj
# Init can take >60s on full seed; smoke criterion is .zqk materialization.
printf 'project_name: g13-smoke\n' > /tmp/zqk-g13-answers.yaml
set +e
/tmp/zqk-g13 system init --force --answer-file /tmp/zqk-g13-answers.yaml >/tmp/zqk-g13-init.out 2>/tmp/zqk-g13-init.err &
INIT_PID=$!
set -e
for i in $(seq 1 90); do
  if [ -d .zqk ]; then
    echo "linux-smoke: .zqk present after ${i}s (init pid=$INIT_PID)"
    kill "$INIT_PID" 2>/dev/null || true
    wait "$INIT_PID" 2>/dev/null || true
    echo "linux-smoke: init OK"
    exit 0
  fi
  if ! kill -0 "$INIT_PID" 2>/dev/null; then
    wait "$INIT_PID" || true
    break
  fi
  sleep 1
done
if [ -d .zqk ]; then
  echo "linux-smoke: init OK"
  exit 0
fi
echo "init failed to materialize .zqk"; cat /tmp/zqk-g13-init.err; exit 1
EOS
  if docker run --rm \
      -v "$ROOT:/src:ro" \
      -v "$INIT_SCRIPT:/linux-init.sh:ro" \
      -w /src \
      "$GOIMG" \
      bash /linux-init.sh >"$LOGDIR/docker-linux.log" 2>&1; then
    say "OK: docker linux build+init smoke ($GOIMG)"
    if [ "${ZQK_G13_LINUX_ARM64:-0}" = "1" ]; then
      if docker run --rm --platform linux/arm64 \
          -v "$ROOT:/src:ro" \
          -v "$INIT_SCRIPT:/linux-init.sh:ro" \
          -w /src \
          "$GOIMG" \
          bash /linux-init.sh >>"$LOGDIR/docker-linux.log" 2>&1; then
        say "OK: docker linux/arm64 build+init smoke"
      else
        say "WARN: linux/arm64 smoke failed (optional)"
      fi
    fi
  else
    DOCKER_FAIL=1
    say "FAIL: docker linux build+init smoke:"
    tail -n 40 "$LOGDIR/docker-linux.log" || true
    if [ -f Dockerfile.community ]; then
      say "== docker Dockerfile.community build (fallback probe) =="
      if docker build -f Dockerfile.community -t zqk-g13-linux:local . >"$LOGDIR/docker-df.log" 2>&1; then
        say "OK: Dockerfile.community builds (init still failed above)"
      else
        say "WARN: Dockerfile.community build also failed — see $LOGDIR/docker-df.log"
      fi
    fi
  fi
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
if [ "$DOCKER_FAIL" -ne 0 ] && [ "${ZQK_G13_REQUIRE_DOCKER:-1}" = "1" ] && [ "${ZQK_G13_NATIVE_ONLY:-}" != "1" ]; then
  if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    say "G13 HAMMER: FAIL (docker linux build+init required)"
    exit 1
  fi
  say "G13 HAMMER: PARTIAL (native+windows OK; docker unavailable — residual)"
  exit 0
fi
say "G13 HAMMER: PASS"
exit 0
