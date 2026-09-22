#!/usr/bin/env bash
# generate-docs-portal.sh — CLI wrapper for static docs portal generation and verification
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PYTHON="${PYTHON:-python3}"

if [[ "${1:-}" == "--verify" ]]; then
  TARBALL="${2:-}"
  if [[ -z "${TARBALL}" || ! -f "${TARBALL}" ]]; then
    echo "Error: nonexistent tarball ${TARBALL}" >&2
    exit 1
  fi
  "${PYTHON}" "${SCRIPT_DIR}/generate_docs_portal.py" --verify "${TARBALL}"
  exit $?
fi

OUT_DIR="${1:-dist-docs}"
"${PYTHON}" "${SCRIPT_DIR}/generate_docs_portal.py" "${OUT_DIR}"

# Create tarball archive for distribution & verification
TARBALL="${OUT_DIR}.tar.gz"
PARENT_DIR="$(dirname "${OUT_DIR}")"
BASE_DIR="$(basename "${OUT_DIR}")"
tar -czf "${TARBALL}" -C "${PARENT_DIR}" "${BASE_DIR}"
echo "📦 Created documentation tarball at ${TARBALL}"
