#!/usr/bin/env bash
# ZQK Showcase Demo Suite: Master Runner
# Runs the official ZQK interactive demonstrations:
# 1. Day-0 Greenfield DX & Instant Agent Orientation
# 2. Crash-Consistency ("Kill -9") & Transactional Resurrection
# 3. Dynamic Membrane & Fail-Closed Prompt Injection Intercept
# 4. Verifiable Multi-Agent Ledger & CISA OpenVEX Attestation
# 5. Object Inspector, Policy Rule Studio & Unified QA Console
#
# Flags:
#   --all / --batch / --non-interactive  Run all demos without interactive pauses
#   --capture                            Capture terminal output to styled SVG screenshots
#   --capture-native-png                 Also capture native desktop PNG screenshots (macOS)
#   --demo <1-5>                         Run a specific demo directly
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
SCREENSHOTS_DIR="${ROOT_DIR}/docs/demos/screenshots"
CAPTURE_PY="${SCRIPT_DIR}/capture_terminal.py"

BOLD='\033[1m'
CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

DO_CAPTURE=0
DO_NATIVE_PNG=0
INTERACTIVE=1

# Parse command line flags
POSITIONAL=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --capture)
      DO_CAPTURE=1
      shift
      ;;
    --capture-native-png)
      DO_CAPTURE=1
      DO_NATIVE_PNG=1
      shift
      ;;
    --batch|--non-interactive|--all)
      INTERACTIVE=0
      shift
      ;;
    --demo)
      POSITIONAL+=("$2")
      shift 2
      ;;
    *)
      POSITIONAL+=("$1")
      shift
      ;;
  esac
done

if [[ "${ZQK_DEMO_CAPTURE:-0}" == "1" ]]; then
  DO_CAPTURE=1
fi

print_header() {
  clear 2>/dev/null || true
  echo -e "${BOLD}═══════════════════════════════════════════════════════════════════════════${NC}"
  echo -e "${BOLD}${CYAN}                ZQK CORE SHOWCASE DEMONSTRATION SUITE                      ${NC}"
  echo -e "${BOLD}═══════════════════════════════════════════════════════════════════════════${NC}"
  echo -e "  Autonomous AI Agent Operating System & Knowledge Kernel"
  echo -e "  ${YELLOW}Principle:${NC} Agents manage the work. ZQK enforces the physics."
  echo -e "${BOLD}═══════════════════════════════════════════════════════════════════════════${NC}\n"
}

run_demo_file() {
  local script_name="$1"
  local demo_id="$2"
  local demo_title="$3"
  local script_path="${SCRIPT_DIR}/${script_name}"

  if [[ "${DO_CAPTURE}" -eq 1 ]]; then
    mkdir -p "${SCREENSHOTS_DIR}"
    local svg_out="${SCREENSHOTS_DIR}/${demo_id}.svg"
    local png_out="${SCREENSHOTS_DIR}/${demo_id}.png"
    local capture_args=("--output-svg" "${svg_out}" "--title" "${demo_title}")
    if [[ "${DO_NATIVE_PNG}" -eq 1 ]]; then
      capture_args+=("--capture-native-png" "--output-png" "${png_out}")
    fi
    python3 "${CAPTURE_PY}" "${capture_args[@]}" bash "${script_path}"
  else
    bash "${script_path}"
  fi
}

run_demo() {
  local num="$1"
  case "$num" in
    1)
      run_demo_file "day_zero_dx_demo.sh" "demo1_dayzero_dx" "zqk — Demo 1: Day-0 Greenfield DX"
      ;;
    2)
      run_demo_file "time_travel_demo.sh" "demo2_kill9_resurrection" "zqk — Demo 2: Kill -9 Resurrection"
      ;;
    3)
      run_demo_file "containment_breach_demo.sh" "demo3_containment_breach" "zqk — Demo 3: Dynamic Membrane Intercept"
      ;;
    4)
      run_demo_file "crypto_audit_demo.sh" "demo4_crypto_audit" "zqk — Demo 4: Cryptographic Ledger & OpenVEX"
      ;;
    5)
      run_demo_file "object_inspector_policy_studio_demo.sh" "demo5_object_inspector" "zqk — Demo 5: Object Inspector & Policy Studio"
      ;;
    *)
      echo -e "${RED}Invalid demo selection: ${num}${NC}"
      exit 1
      ;;
  esac
}

run_all() {
  print_header
  echo -e "${BOLD}Executing full 5-part authentic showcase sequence...${NC}"
  if [[ "${DO_CAPTURE}" -eq 1 ]]; then
    echo -e "  ${CYAN}Screenshot capture active: writing SVG assets to docs/demos/screenshots/${NC}"
  fi
  echo ""
  
  echo -e "${BOLD}▶ [1/5] DAY-0 GREENFIELD DEVELOPER & AGENT EXPERIENCE${NC}"
  run_demo 1
  
  if [[ "$INTERACTIVE" -eq 1 ]]; then
    echo -e "\n${BOLD}Press Enter to proceed to Demo 2 (or Ctrl+C to stop)...${NC}"
    read -r _ || true
  fi
  
  echo -e "\n${BOLD}▶ [2/5] CRASH-CONSISTENCY & KILL -9 RESURRECTION${NC}"
  run_demo 2
  
  if [[ "$INTERACTIVE" -eq 1 ]]; then
    echo -e "\n${BOLD}Press Enter to proceed to Demo 3 (or Ctrl+C to stop)...${NC}"
    read -r _ || true
  fi
  
  echo -e "\n${BOLD}▶ [3/5] DYNAMIC MEMBRANE & FAIL-CLOSED PROMPT INJECTION INTERCEPT${NC}"
  run_demo 3
  
  if [[ "$INTERACTIVE" -eq 1 ]]; then
    echo -e "\n${BOLD}Press Enter to proceed to Demo 4 (or Ctrl+C to stop)...${NC}"
    read -r _ || true
  fi
  
  echo -e "\n${BOLD}▶ [4/5] VERIFIABLE CRYPTOGRAPHIC LEDGER & OPENVEX ATTESTATION${NC}"
  run_demo 4

  if [[ "$INTERACTIVE" -eq 1 ]]; then
    echo -e "\n${BOLD}Press Enter to proceed to Demo 5 (or Ctrl+C to stop)...${NC}"
    read -r _ || true
  fi

  echo -e "\n${BOLD}▶ [5/5] OBJECT INSPECTOR, LIVE POLICY STUDIO & UNIFIED QA CONSOLE${NC}"
  run_demo 5

  echo -e "\n${BOLD}═══════════════════════════════════════════════════════════════════════════${NC}"
  echo -e "${BOLD}${GREEN}✔ ALL 5 SHOWCASE DEMOS COMPLETED SUCCESSFULLY${NC}"
  if [[ "${DO_CAPTURE}" -eq 1 ]]; then
    echo -e "  Screenshots saved to: ${CYAN}${SCREENSHOTS_DIR}/${NC}"
  fi
  echo -e "${BOLD}═══════════════════════════════════════════════════════════════════════════${NC}\n"
}

if [[ ${#POSITIONAL[@]} -gt 0 ]]; then
  TARGET="${POSITIONAL[0]}"
  if [[ "${TARGET}" =~ ^[1-5]$ ]]; then
    run_demo "${TARGET}"
    exit 0
  fi
fi

if [[ "$INTERACTIVE" -eq 0 ]]; then
  run_all
  exit 0
fi

# Interactive menu
print_header
echo -e "Select a showcase demo to execute:\n"
echo -e "  ${BOLD}[1]${NC} Day-0 Greenfield DX     (Instant setup, starter graph, health verification)"
echo -e "  ${BOLD}[2]${NC} Crash-Consistency       (Real kill -9, WAL durability, zero data loss)"
echo -e "  ${BOLD}[3]${NC} Dynamic Membrane        (Live MCP server, prompt injection trapping)"
echo -e "  ${BOLD}[4]${NC} Cryptographic Audit     (Multi-agent seating, CISA OpenVEX attestation)"
echo -e "  ${BOLD}[5]${NC} Object Inspector & QA   (Dual human/agent views, Policy Studio, DoD QA)"
echo -e "  ${BOLD}[A]${NC} Run All Demonstrations"
echo -e "  ${BOLD}[Q]${NC} Quit\n"

read -p "Enter choice [1-5, A, Q]: " -n 1 -r CHOICE
echo ""

case "${CHOICE}" in
  1|2|3|4|5)
    run_demo "${CHOICE}"
    ;;
  [aA])
    run_all
    ;;
  [qQ])
    echo "Exiting."
    exit 0
    ;;
  *)
    echo "Invalid selection."
    exit 1
    ;;
esac
