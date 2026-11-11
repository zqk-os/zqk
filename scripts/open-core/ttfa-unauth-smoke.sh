#!/usr/bin/env bash
# ttfa-unauth-smoke.sh — Stranger TTFA: install→init→smoke without GITHUB_TOKEN.
#
# Measures wall time for init + whats-next (smoke). Fails if measured segment > 120s.
# Install uses local checkout source/goinstall with tokens unset (and ZQK_INSTALL_UNAUTH=1).
#
# Usage (from repo root or any cwd):
#   ./scripts/open-core/ttfa-unauth-smoke.sh
#   TTFA_MAX_SECONDS=90 ./scripts/open-core/ttfa-unauth-smoke.sh
#
# Env:
#   TTFA_MAX_SECONDS   Max wall seconds for measured init+smoke (default 120)
#   ZQK_INSTALL_METHOD source|goinstall|auto (default source)
#   KEEP_TTFA_WORKDIR  If 1, print and keep the temp workdir
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
MAX_SECONDS="${TTFA_MAX_SECONDS:-120}"
INSTALL_METHOD="${ZQK_INSTALL_METHOD:-source}"

# Stranger path: never inherit host tokens
unset GITHUB_TOKEN GH_TOKEN || true
export ZQK_INSTALL_UNAUTH=1

WORK_DIR="$(mktemp -d -t zqk-ttfa-XXXXXX)"
BIN_DIR="${WORK_DIR}/bin"
PROJECT_DIR="${WORK_DIR}/greenfield"
mkdir -p "$BIN_DIR" "$PROJECT_DIR"

cleanup() {
  if [ "${KEEP_TTFA_WORKDIR:-0}" = "1" ]; then
    echo "KEEP_TTFA_WORKDIR=1 — left ${WORK_DIR}" >&2
  else
    rm -rf "$WORK_DIR"
  fi
}
trap cleanup EXIT

echo "=== TTFA unauth smoke ==="
echo "repo:            ${REPO_ROOT}"
echo "workdir:         ${WORK_DIR}"
echo "install_method:  ${INSTALL_METHOD}"
echo "max_seconds:     ${MAX_SECONDS}"
echo "GITHUB_TOKEN set? ${GITHUB_TOKEN+yes}${GITHUB_TOKEN-no}"
echo "GH_TOKEN set?     ${GH_TOKEN+yes}${GH_TOKEN-no}"
echo ""

# ---------------------------------------------------------------------------
# Install (not counted toward 120s gate — cold module download varies)
# ---------------------------------------------------------------------------
echo "── Install (${INSTALL_METHOD}, unauth) ──"
cd "$REPO_ROOT"
ZQK_INSTALL_DIR="$BIN_DIR" \
  ZQK_INSTALL_METHOD="$INSTALL_METHOD" \
  ZQK_INSTALL_UNAUTH=1 \
  ZQK_MODE=fast \
  sh "${REPO_ROOT}/scripts/install.sh" --headless --no-sudo --prefix="${BIN_DIR}"

export PATH="${BIN_DIR}:${PATH}"
ZQK_BIN="${BIN_DIR}/zqk"
if [ ! -x "$ZQK_BIN" ]; then
  echo "FAIL: zqk binary missing after install at ${ZQK_BIN}" >&2
  exit 1
fi
echo "✓ installed: $($ZQK_BIN version 2>/dev/null || echo ok)"
echo ""

# ---------------------------------------------------------------------------
# Measured segment: init + smoke
# ---------------------------------------------------------------------------
echo "── Measured: zqk init + zqk workflow whats-next ──"
START_EPOCH="$(date +%s)"
START_NS="$(date +%s%N 2>/dev/null || echo "")"

(
  cd "$PROJECT_DIR"
  # Isolate from studio/host kernel
  unset ZQK_PROJECT_ROOT || true
  "$ZQK_BIN" init
  "$ZQK_BIN" workflow whats-next --skip-measure 2>/dev/null \
    || "$ZQK_BIN" workflow whats-next
)

END_EPOCH="$(date +%s)"
ELAPSED=$((END_EPOCH - START_EPOCH))
if [ -n "$START_NS" ] && [[ "$START_NS" =~ ^[0-9]+$ ]]; then
  END_NS="$(date +%s%N 2>/dev/null || echo "")"
  if [ -n "$END_NS" ] && [[ "$END_NS" =~ ^[0-9]+$ ]]; then
    # Prefer integer milliseconds when %N available
    ELAPSED_MS=$(( (END_NS - START_NS) / 1000000 ))
    ELAPSED=$(( (ELAPSED_MS + 999) / 1000 ))
    echo "elapsed_ms=${ELAPSED_MS}"
  fi
fi

echo "elapsed_seconds=${ELAPSED}"
echo "threshold_seconds=${MAX_SECONDS}"

if [ "$ELAPSED" -gt "$MAX_SECONDS" ]; then
  echo "FAIL: TTFA measured segment ${ELAPSED}s > ${MAX_SECONDS}s" >&2
  exit 1
fi

echo "PASS: TTFA unauth init+smoke ${ELAPSED}s ≤ ${MAX_SECONDS}s (no GITHUB_TOKEN)"
# Machine-readable footer for evidence capture
echo "TTFA_RESULT ok elapsed_seconds=${ELAPSED} max_seconds=${MAX_SECONDS} method=${INSTALL_METHOD}"
