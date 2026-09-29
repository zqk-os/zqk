#!/usr/bin/env bash
# deploy-docs-portal.sh — Compile and deploy static documentation portal from ZQK Core to zqk-os/zqk-docs (docs.zqk.dev)
#
# Usage:
#   ./scripts/deploy-docs-portal.sh [--dry-run]
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
BUILD_DIR="/tmp/zqk-docs-build"
DOCS_REPO="git@github.com:zqk-os/zqk-docs.git"
CUSTOM_DOMAIN="docs.zqk.dev"

DRY_RUN=0
if [[ "${1:-}" == "--dry-run" ]]; then
  DRY_RUN=1
  echo "🔍 [DRY RUN] Will generate and verify portal without pushing to remote."
fi

echo "🚀 [1/4] Generating static documentation portal into ${BUILD_DIR} from ZQK Core..."
rm -rf "${BUILD_DIR}"
python3 "${SCRIPT_DIR}/generate_docs_portal.py" "${BUILD_DIR}"

echo "⚙️ [2/4] Injecting GitHub Pages CNAME and .nojekyll markers..."
echo "${CUSTOM_DOMAIN}" > "${BUILD_DIR}/CNAME"
touch "${BUILD_DIR}/.nojekyll"

DOC_COUNT=$(find "${BUILD_DIR}" -name "*.html" | wc -l | tr -d ' ')
echo "✅ Compiled ${DOC_COUNT} HTML documentation pages."

if [[ "${DRY_RUN}" -eq 1 ]]; then
  echo "🏁 [DRY RUN COMPLETE] Portal verified at ${BUILD_DIR}."
  exit 0
fi

echo "📦 [3/4] Preparing Git deployment to ${DOCS_REPO} (main)..."
cd "${BUILD_DIR}"
git init -b main
git config user.name "zqk-bot"
git config user.email "bot@zqkos.com"
git add .

COMMIT_SHA=$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null || echo "core")
git commit -m "docs: publish canonical open-core documentation portal [commit ${COMMIT_SHA}]"

git remote add origin "${DOCS_REPO}"

echo "📡 [4/4] Pushing to ${DOCS_REPO}..."
git push --force origin main

echo "✨ Open-Core Documentation portal successfully published to https://${CUSTOM_DOMAIN}"
