#!/bin/sh
# scripts/install_test.sh — Automated tests for standalone zero-dependency installer
# Verifies standalone zero-dependency installer flags and headless execution
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
INSTALL_SCRIPT="${REPO_ROOT}/scripts/install.sh"

echo "=== Running Standalone Installer Test Suite ==="

# 1. Syntax check via sh -n
echo "Checking shell syntax (sh -n)..."
sh -n "$INSTALL_SCRIPT"
echo "✓ Syntax OK"

# 2. Inspect script content for required arguments and invariants
echo "Checking argument support and static invariants..."
for pattern in \
  "--standalone|-s)" \
  "--headless)" \
  "--fast|-f)" \
  "--walkthrough|-w)" \
  "INSTALL_DIR=" \
  "zgrep" \
  "10-Second Value Prop:" \
  "zqk mcp install" \
  "to supercharge Cursor / Claude Code with sub-15ms persistent memory."; do
  if ! grep -Fq -- "$pattern" "$INSTALL_SCRIPT"; then
    echo "❌ Missing required pattern in install.sh: $pattern" >&2
    exit 1
  fi
done
echo "✓ Static invariants verified"

# 3. Test execution with --standalone in an isolated temp directory
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_DIR"' EXIT

echo "Testing execution: scripts/install.sh --standalone..."
ZQK_INSTALL_DIR="${TEST_DIR}/bin" sh "$INSTALL_SCRIPT" --standalone > "${TEST_DIR}/output.log" 2>&1

if [ ! -f "${TEST_DIR}/bin/zqk" ]; then
  echo "❌ Expected ${TEST_DIR}/bin/zqk to be installed" >&2
  cat "${TEST_DIR}/output.log"
  exit 1
fi

if [ ! -L "${TEST_DIR}/bin/zgrep" ] && [ ! -f "${TEST_DIR}/bin/zgrep" ]; then
  echo "❌ Expected ${TEST_DIR}/bin/zgrep symlink to be created" >&2
  cat "${TEST_DIR}/output.log"
  exit 1
fi

# Verify banner and guidance in output
if ! grep -q "ZQK: Git for AI Agent Memory & Task Truth" "${TEST_DIR}/output.log"; then
  echo "❌ Banner missing in installer output" >&2
  cat "${TEST_DIR}/output.log"
  exit 1
fi

if ! grep -q "10-Second Value Prop:" "${TEST_DIR}/output.log"; then
  echo "❌ 10-Second Value Prop missing in installer output" >&2
  cat "${TEST_DIR}/output.log"
  exit 1
fi

if ! grep -q "Next step: run zqk mcp install to supercharge Cursor / Claude Code with sub-15ms persistent memory." "${TEST_DIR}/output.log"; then
  echo "❌ Next step guidance missing in installer output" >&2
  cat "${TEST_DIR}/output.log"
  exit 1
fi

echo "✓ Standalone execution verified successfully"

# 4. Test execution with short flag -s
TEST_DIR2="$(mktemp -d)"
echo "Testing execution: scripts/install.sh -s..."
ZQK_INSTALL_DIR="${TEST_DIR2}/bin" sh "$INSTALL_SCRIPT" -s > "${TEST_DIR2}/output.log" 2>&1
if [ ! -f "${TEST_DIR2}/bin/zqk" ] || [ ! -e "${TEST_DIR2}/bin/zgrep" ]; then
  echo "❌ Expected -s flag to install zqk and zgrep" >&2
  cat "${TEST_DIR2}/output.log"
  rm -rf "$TEST_DIR2"
  exit 1
fi
rm -rf "$TEST_DIR2"
echo "✓ Short flag -s execution verified successfully"

# 5. Test non-interactive pipe execution (curl -fsSL ... | sh)
TEST_DIR3="$(mktemp -d)"
echo "Testing piped execution: sh scripts/install.sh < /dev/null..."
ZQK_INSTALL_DIR="${TEST_DIR3}/bin" sh "$INSTALL_SCRIPT" < /dev/null > "${TEST_DIR3}/output.log" 2>&1
if [ ! -f "${TEST_DIR3}/bin/zqk" ] || [ ! -e "${TEST_DIR3}/bin/zgrep" ]; then
  echo "❌ Expected piped execution to install zqk and zgrep without blocking" >&2
  cat "${TEST_DIR3}/output.log"
  rm -rf "$TEST_DIR3"
  exit 1
fi
rm -rf "$TEST_DIR3"
echo "✓ Piped headless execution verified successfully"

echo ""
echo "🎉 All standalone installer tests passed successfully!"
exit 0
