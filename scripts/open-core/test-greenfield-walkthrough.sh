#!/usr/bin/env bash
# test-greenfield-walkthrough.sh — End-to-end greenfield startup walkthrough tester.
#
# Simulates a day-zero stranger experience in an isolated temporary directory
# with an isolated HOME (no operator ~/.cursor/mcp.json studio pin bleed) and
# fail-closed pack-ontology scrub on whats-next JSON.
#
# Uses either a specified version from GitHub Releases (using gh auth if private)
# or a specified local binary.
#
# Usage:
#   ./scripts/open-core/test-greenfield-walkthrough.sh [version-tag]
#   ./scripts/open-core/test-greenfield-walkthrough.sh v0.1.0-beta.4
#   ZQK_BIN=/path/to/zqk ./scripts/open-core/test-greenfield-walkthrough.sh
#
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger greenfield residual pack-free honesty

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
HOME_DIR="${WORK_DIR}/home"
SCRUB_DIR="${WORK_DIR}/scrub"
mkdir -p "$BIN_DIR" "$PROJECT_DIR" "$HOME_DIR" "$SCRUB_DIR"
TEST_ZQK=""

# Avoid studio kernel bleed-through into the stranger cell.
# Isolated HOME so operator ~/.cursor/mcp.json (studio pin) cannot hijack
# node_sovereignty and poison pack-deny scans with absolute studio paths.
apply_stranger_env() {
  unset ZQK_PROJECT_ROOT ZQK_CURRENT_ROOT ZQK_ENABLE_STUDIO_PACK_TOOLS ZQK_STUDIO_DOGFOOD 2>/dev/null || true
  export HOME="${HOME_DIR}"
  export ZQK_HOME="${HOME_DIR}"
}

cleanup_greenfield() {
  # Temp-epoch ambient daemons must not leak after walkthrough (OPEN_CORE_PUBLIC_RELEASE_HYGIENE).
  if [ "${KEEP_GREENFIELD_WORKDIR:-0}" = "1" ]; then
    echo "KEEP_GREENFIELD_WORKDIR=1 — left ${WORK_DIR}" >&2
    return 0
  fi
  if [ -n "${TEST_ZQK:-}" ] && [ -x "${TEST_ZQK}" ] && [ -d "${PROJECT_DIR:-}" ]; then
    (
      cd "${PROJECT_DIR}"
      unset ZQK_CURRENT_ROOT 2>/dev/null || true
      export ZQK_PROJECT_ROOT="${PROJECT_DIR}"
      export HOME="${HOME_DIR:-${HOME:-}}"
      export ZQK_HOME="${HOME_DIR:-${ZQK_HOME:-}}"
      "${TEST_ZQK}" ambient stop >/dev/null 2>&1 || true
    )
  fi
  pkill -f "${WORK_DIR}" >/dev/null 2>&1 || true
  chmod -R u+w "${WORK_DIR}" >/dev/null 2>&1 || true
  rm -rf "${WORK_DIR}" >/dev/null 2>&1 || true
  return 0
}
trap cleanup_greenfield EXIT

echo -e "${BLUE}ℹ Sandbox Workspace:${RESET} ${WORK_DIR}"
echo -e "${BLUE}ℹ Isolated HOME:${RESET}     ${HOME_DIR}"
echo -e "${BLUE}ℹ Target Version:${RESET}     ${VERSION}"
echo ""

# ---------------------------------------------------------------------------
# Step 1: Install / Locate Binary
# ---------------------------------------------------------------------------
echo -e "${BOLD}${YELLOW}── Step 1: Install Binary (${VERSION}) ──${RESET}"

# Prefer an explicit binary, then a tree-local bin that already speaks `init`
# (community/PC), then dist-community archives, then installer. Local
# install.sh can build a studio stub that lacks top-level `init` — fail closed
# after selection rather than treating a missing membrane as success.
bin_supports_init() {
  local cand="$1"
  [ -x "$cand" ] || return 1
  "$cand" init --help >/dev/null 2>&1 || "$cand" system init --help >/dev/null 2>&1
}

if [ -n "${ZQK_BIN:-}" ] && [ -x "$ZQK_BIN" ]; then
  echo -e "  Using provided binary: ${ZQK_BIN}"
  cp "$ZQK_BIN" "${BIN_DIR}/zqk"
elif bin_supports_init "${REPO_ROOT}/bin/zqk"; then
  echo -e "  Using tree-local binary with init: ${REPO_ROOT}/bin/zqk"
  cp "${REPO_ROOT}/bin/zqk" "${BIN_DIR}/zqk"
elif [ -f "${REPO_ROOT}/dist-community/zqk_${VERSION#v}_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/').tar.gz" ]; then
  echo -e "  Unpacking from local dist-community release archive..."
  ARCHIVE="${REPO_ROOT}/dist-community/zqk_${VERSION#v}_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/').tar.gz"
  tar -xzf "$ARCHIVE" -C "$BIN_DIR" --strip-components=1
  if [ -f "${BIN_DIR}/zqk-community" ] && [ ! -f "${BIN_DIR}/zqk" ]; then
    cp "${BIN_DIR}/zqk-community" "${BIN_DIR}/zqk"
    chmod +x "${BIN_DIR}/zqk"
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

if ! bin_supports_init "$TEST_ZQK"; then
  echo -e "❌ Selected binary lacks \`zqk init\` / \`zqk system init\` (not stranger-ready). Set ZQK_BIN to a community binary." >&2
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
  apply_stranger_env
  echo "  Running 'zqk init' in fresh directory: ${PROJECT_DIR}"
  if "$TEST_ZQK" init --help >/dev/null 2>&1; then
    "$TEST_ZQK" init
  else
    "$TEST_ZQK" system init
  fi
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
  apply_stranger_env
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
  apply_stranger_env
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
  apply_stranger_env
  "$TEST_ZQK" mcp install >"${SCRUB_DIR}/mcp-install.out" 2>&1
  cat "${SCRUB_DIR}/mcp-install.out"
)
# Fail-closed: mcp install text must not pin the studio tree.
if grep -Eiq 'zqke-pack-composition' "${SCRUB_DIR}/mcp-install.out" 2>/dev/null; then
  echo -e "❌ Studio/pack path leaked into mcp install output" >&2
  exit 1
fi
echo -e "${GREEN}✓ MCP server configuration registered${RESET}"
echo ""

# ---------------------------------------------------------------------------
# Step 6: Discover Intent & Tasks (zqk workflow whats-next)
# ---------------------------------------------------------------------------
echo -e "${BOLD}${YELLOW}── Step 6: Discover Tasks (zqk workflow whats-next) ──${RESET}"
(
  cd "$PROJECT_DIR"
  apply_stranger_env
  # Human-facing walkthrough surface
  "$TEST_ZQK" workflow whats-next
  # Machine scrub surface (bounded JSON; skip measure for determinism)
  "$TEST_ZQK" workflow whats-next --format json --skip-measure >"${SCRUB_DIR}/whats-next.json"
)

[ -s "${SCRUB_DIR}/whats-next.json" ] || {
  echo -e "❌ whats-next JSON scrub payload empty" >&2
  exit 1
}

python3 - "${SCRUB_DIR}/whats-next.json" <<'PY'
import json, re, sys

path = sys.argv[1]
with open(path, encoding="utf-8") as f:
    raw = f.read()
try:
    d = json.loads(raw)
except json.JSONDecodeError as e:
    print(f"FAIL: whats-next output is not JSON: {e}", file=sys.stderr)
    sys.exit(1)

pp = d.get("priority_plan") or {}
pid = pp.get("id") if isinstance(pp, dict) else pp
want = "PRI-STARTER-COMMUNITY-001"
if pid != want:
    print(f"FAIL: expected priority_plan {want}, got {pid!r}", file=sys.stderr)
    sys.exit(1)

# Operator-host diagnostics (mcp_pin / node_sovereignty paths) are not product ontology.
scrub = dict(d)
ka = scrub.get("kernel_ambience")
if isinstance(ka, dict):
    ka = dict(ka)
    ka.pop("node_sovereignty", None)
    scrub["kernel_ambience"] = ka
blob = json.dumps(scrub, sort_keys=True)

# Sealed lead ids split so payload nanos-hex grep does not false-positive this gate.
sealed_pris = [
    "PRI-" + "1794405067761890000-7b6d3831",  # sealed greenfield lead
    "PRI-" + "1794404844729744000-6a42dca7",  # sealed seed lead
    "PRI-" + "1794405488622492000-90a0a917",  # sealed MCP-ensure lead
    "PRI-" + "1794415800074969000-ce0d2e20",  # greenfield residual follow-on lead
]
deny = re.compile(
    r"(?i)"
    r"(PER-ORCH-|PER-STUDIO-|ASK-STUDIO-|ASK-CAP-|"
    r"mmorch|ORCHESTRATE_PLAN|"
    r"get_current_priority_plan|get_priority_plan_items|"
    r"get_current_backlog_item|get_next_backlog_item|"
    r"cap_orchestrat|cap_night_duty|cap_background|"
    r"paste_applescript|print_cursor_paste|print_ide_paste|zqk_cursor|"
    r"align-cache|align_cache|"
    + "|".join(re.escape(s) for s in sealed_pris)
    + r"|"
    r"zqke-pack-composition)"
)
hit = deny.search(blob)
if hit:
    print(f"FAIL: pack ontology leakage in whats-next payload near {hit.group(0)!r}", file=sys.stderr)
    sys.exit(1)

print(f"PASS: walkthrough whats-next lead is {want} (pack-free under isolated HOME)")
PY

echo -e "${GREEN}✓ Autonomous task discovery verified (pack-free)${RESET}"
echo ""

# ---------------------------------------------------------------------------
# Step 7: Execute Autonomous Loop (zqk do --dry-run)
# ---------------------------------------------------------------------------
echo -e "${BOLD}${YELLOW}── Step 7: Autonomous Loop (zqk do --dry-run) ──${RESET}"
(
  cd "$PROJECT_DIR"
  apply_stranger_env
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
echo -e "  ${BOLD}export HOME=\"${HOME_DIR}\"${RESET}"
echo ""
