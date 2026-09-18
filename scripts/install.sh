#!/bin/sh
# Build this checkout. There is no public GitHub release yet.
# TRACK: TDE-1789699310016987000-5703b344 — do not curl/clone lanceman/zqk or zqk-os until ACK.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$ROOT"

if [ "${ZQK_INSTALL_METHOD:-}" = "binary" ] || [ "${ZQK_INSTALL_METHOD:-}" = "goinstall" ]; then
  echo "install.sh: no public release or module yet. Use: ./scripts/install.sh" >&2
  echo "That runs make in this tree." >&2
  exit 2
fi

command -v go >/dev/null 2>&1 || {
  echo "Go toolchain not found. Install from https://go.dev/dl/" >&2
  exit 1
}

make

BIN=""
for cand in bin/zqk bin/zcom; do
  if [ -x "$ROOT/$cand" ]; then
    BIN="$ROOT/$cand"
    break
  fi
done
if [ -z "$BIN" ]; then
  echo "make finished but no bin/zqk or bin/zcom was produced" >&2
  exit 1
fi

echo ""
echo "Built ${BIN}"
echo "Run it from ${ROOT}. Do not export a project-root environment variable."
echo ""
echo "  ${BIN} --version"
echo "  ${BIN} system agent-onboard --format json"
echo "  ${BIN} workflow whats-next --format json"
