#!/usr/bin/env bash
# test-greenfield-walkthrough.sh — End-to-end greenfield startup walkthrough tester.
#
# Simulates a day-zero stranger experience in an isolated temporary directory.
# Uses either a specified version from GitHub Releases (using gh auth if private)
# or a specified local binary.
#
# Usage:
#   ./scripts/test-greenfield-walkthrough.sh [version-tag]
#   ./scripts/test-greenfield-walkthrough.sh v0.1.0-beta.4
#   ZQK_BIN=/path/to/zqk ./scripts/test-greenfield-walkthrough.sh

set -euo pipefail

VERSION="${1:-v0.1.0-beta.4}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Colors for terminal output
BOLD="\033[1m"
GREEN="\033[32m"
BLUE="\033[34m"
YELLOW="\033[33m"
CYAN="\033[36m"
RESET="\033[0m"

echo -e "${BOLD}${CYAN}════════════════════════════════════════════════════════════════${RESET}"
echo -e "${BOLD}${CYAN}      ZQK Greenfield End-to-End Walkthrough Tester              ${RESET}"
echo -e "${BOLD}${CYAN}════════════════════════════════════════════════════════════════${RESET}"
echo ""

WORK_DIR="$(mktemp -d -t zqk-greenfield-XXXXXX)"
BIN_DIR="${WORK_DIR}/bin"
PROJECT_DIR="${WORK_DIR}/my-greenfield-project"
mkdir -p "$BIN_DIR" "$PROJECT_DIR"

echo -e "${BLUE}ℹ Sandbox Workspace:${RESET} ${WORK_DIR}"
echo -e "${BLUE}ℹ Target Version:${RESET}     ${VERSION}"
echo ""

# ---------------------------------------------------------------------------
# Step 1: Install / Locate Binary
# ---------------------------------------------------------------------------
echo -e "${BOLD}${YELLOW}── Step 1: Install Binary (${VERSION}) ──${RESET}"

if [ -n "${ZQK_BIN:-}" ] && [ -x "$ZQK_BIN" ]; then
  echo -e "  Using provided binary: ${ZQK_BIN}"
  cp "$ZQK_BIN" "${BIN_DIR}/zqk"
elif [ -f "${REPO_ROOT}/dist-community/zqk_${VERSION#v}_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/').tar.gz" ]; then
  echo -e "  Unpacking from local dist-community release archive..."
  ARCHIVE="${REPO_ROOT}/dist-community/zqk_${VERSION#v}_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/').tar.gz"
  tar -xzf "$ARCHIVE" -C "$BIN_DIR" --strip-components=1
  if [ -f "${BIN_DIR}/zqk-community" ] && [ ! -f "${BIN_DIR}/zqk" ]; then
    cp "${BIN_DIR}/zqk-community" "${BIN_DIR}/zqk"
  fi
  chmod +x "${BIN_DIR}/zqk"
else
  echo -e "  Invoking installer (authenticating via gh/GITHUB_TOKEN if private)..."
  ZQK_INSTALL_DIR="$BIN_DIR" "${REPO_ROOT}/scripts/install.sh" "$VERSION"
fi

export PATH="${BIN_DIR}:${PATH}"
TEST_ZQK="${BIN_DIR}/zqk"

if ! command -v "$TEST_ZQK" >/dev/null 2>&1; then
  echo -e "❌ Failed to locate zqk binary at ${TEST_ZQK}" >&2
  exit 1
fi

echo -e "${GREEN}✓ Binary ready:${RESET} $($TEST_ZQK version 2>/dev/null || echo 'installed')"
echo ""

# ---------------------------------------------------------------------------
# Step 2: Seed Sovereign Cell (zqk init)
# ---------------------------------------------------------------------------
echo -e "${BOLD}${YELLOW}── Step 2: Seed Sovereign Cell (zqk init) ──${RESET}"
(
  cd "$PROJECT_DIR"
  echo "  Running 'zqk init' in fresh directory: ${PROJECT_DIR}"
  "$TEST_ZQK" init
)

if [ -d "${PROJECT_DIR}/.zqk" ]; then
  echo -e "${GREEN}✓ Membrane initialized:${RESET} ${PROJECT_DIR}/.zqk created successfully"
else
  echo -e "❌ .zqk directory was not created!" >&2
  exit 1
fi
echo ""

# ---------------------------------------------------------------------------
# Step 3: Run Quickstart Walkthrough (zqk quickstart)
# ---------------------------------------------------------------------------
echo -e "${BOLD}${YELLOW}── Step 3: First-Run Quickstart Hint (zqk quickstart) ──${RESET}"
(
  cd "$PROJECT_DIR"
  "$TEST_ZQK" quickstart
)
echo -e "${GREEN}✓ Quickstart walkthrough executed successfully${RESET}"
echo ""

# ---------------------------------------------------------------------------
# Step 4: Seat AI Agent & Detect IDE (zqk system agent-onboard)
# ---------------------------------------------------------------------------
echo -e "${BOLD}${YELLOW}── Step 4: Seat AI Agent (zqk system agent-onboard) ──${RESET}"
(
  cd "$PROJECT_DIR"
  "$TEST_ZQK" system agent-onboard
)
echo -e "${GREEN}✓ Agent seated and environment primed${RESET}"
echo ""

# ---------------------------------------------------------------------------
# Step 5: Install MCP Configuration (zqk mcp install)
# ---------------------------------------------------------------------------
echo -e "${BOLD}${YELLOW}── Step 5: Connect MCP (zqk mcp install) ──${RESET}"
(
  cd "$PROJECT_DIR"
  "$TEST_ZQK" mcp install
)
echo -e "${GREEN}✓ MCP server configuration registered${RESET}"
echo ""

# ---------------------------------------------------------------------------
# Step 6: Discover Intent & Tasks (zqk workflow whats-next)
# ---------------------------------------------------------------------------
echo -e "${BOLD}${YELLOW}── Step 6: Discover Tasks (zqk workflow whats-next) ──${RESET}"
(
  cd "$PROJECT_DIR"
  "$TEST_ZQK" workflow whats-next
)
echo -e "${GREEN}✓ Autonomous task discovery verified${RESET}"
echo ""

# ---------------------------------------------------------------------------
# Step 7: Execute Autonomous Loop (zqk do --dry-run)
# ---------------------------------------------------------------------------
echo -e "${BOLD}${YELLOW}── Step 7: Autonomous Loop (zqk do --dry-run) ──${RESET}"
(
  cd "$PROJECT_DIR"
  "$TEST_ZQK" do --dry-run
)
echo -e "${GREEN}✓ Task claim and execution loop verified${RESET}"
echo ""

# ---------------------------------------------------------------------------
# Summary & Next Steps
# ---------------------------------------------------------------------------
echo -e "${BOLD}${CYAN}════════════════════════════════════════════════════════════════${RESET}"
echo -e "${BOLD}${GREEN}      ✅ Greenfield End-to-End Walkthrough PASSED!              ${RESET}"
echo -e "${BOLD}${CYAN}════════════════════════════════════════════════════════════════${RESET}"
echo ""
echo -e "You can inspect or interact with the test sandbox at:"
echo -e "  ${BOLD}cd ${PROJECT_DIR}${RESET}"
echo -e "  ${BOLD}export PATH=\"${BIN_DIR}:\$PATH\"${RESET}"
echo ""
