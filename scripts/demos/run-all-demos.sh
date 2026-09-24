#!/usr/bin/env bash
# ZQK Showcase Demo Suite: Master Runner
# Runs the official ZQK killer demos demonstrating:
# 1. Day-0 Developer Experience & Instant Greenfield Initialization
# 2. Crash-Consistency ("Kill -9") & Transactional Resurrection
# 3. Dynamic Membrane & Fail-Closed Prompt Injection Intercept
# 4. Verifiable Multi-Agent Ledger & CISA OpenVEX Attestation
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"

BOLD='\033[1m'
CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

print_header() {
  clear 2>/dev/null || true
  echo -e "${BOLD}═══════════════════════════════════════════════════════════════════════════${NC}"
  echo -e "${BOLD}${CYAN}                ZQK CORE SHOWCASE DEMONSTRATION SUITE                      ${NC}"
  echo -e "${BOLD}═══════════════════════════════════════════════════════════════════════════${NC}"
  echo -e "  Autonomous AI Agent Operating System & Knowledge Kernel"
  echo -e "  ${YELLOW}Principle:${NC} Agents manage the work. ZQK enforces the physics."
  echo -e "${BOLD}═══════════════════════════════════════════════════════════════════════════${NC}\n"
}

run_demo() {
  local num="$1"
  case "$num" in
    1)
      bash "${SCRIPT_DIR}/day_zero_dx_demo.sh"
      ;;
    2)
      bash "${SCRIPT_DIR}/time_travel_demo.sh"
      ;;
    3)
      bash "${SCRIPT_DIR}/containment_breach_demo.sh"
      ;;
    4)
      bash "${SCRIPT_DIR}/crypto_audit_demo.sh"
      ;;
    5)
      bash "${SCRIPT_DIR}/object_inspector_policy_studio_demo.sh"
      ;;
    *)
      echo -e "${RED}Invalid demo selection: ${num}${NC}"
      exit 1
      ;;
  esac
}

run_all() {
  local interactive=1
  if [[ "${1:-}" == "--batch" || "${1:-}" == "--non-interactive" || "${NON_INTERACTIVE:-0}" == "1" ]]; then
    interactive=0
  fi

  print_header
  echo -e "${BOLD}Executing full 5-part showcase sequence...${NC}\n"
  
  echo -e "${BOLD}▶ [1/5] DAY-0 GREENFIELD DEVELOPER EXPERIENCE${NC}"
  run_demo 1
  
  if [[ "$interactive" -eq 1 ]]; then
    echo -e "\n${BOLD}Press Enter to proceed to Demo 2 (or Ctrl+C to stop)...${NC}"
    read -r _ || true
  fi
  
  echo -e "\n${BOLD}▶ [2/5] CRASH-CONSISTENCY & KILL -9 RESURRECTION${NC}"
  run_demo 2
  
  if [[ "$interactive" -eq 1 ]]; then
    echo -e "\n${BOLD}Press Enter to proceed to Demo 3 (or Ctrl+C to stop)...${NC}"
    read -r _ || true
  fi
  
  echo -e "\n${BOLD}▶ [3/5] DYNAMIC MEMBRANE & PROMPT INJECTION INTERCEPT${NC}"
  run_demo 3
  
  if [[ "$interactive" -eq 1 ]]; then
    echo -e "\n${BOLD}Press Enter to proceed to Demo 4 (or Ctrl+C to stop)...${NC}"
    read -r _ || true
  fi
  
  echo -e "\n${BOLD}▶ [4/5] CRYPTOGRAPHIC AUDIT & OPENVEX ATTESTATION${NC}"
  run_demo 4

  if [[ "$interactive" -eq 1 ]]; then
    echo -e "\n${BOLD}Press Enter to proceed to Demo 5 (or Ctrl+C to stop)...${NC}"
    read -r _ || true
  fi

  echo -e "\n${BOLD}▶ [5/5] OBJECT INSPECTOR, POLICY STUDIO & UNIFIED QA CONSOLE${NC}"
  run_demo 5

  echo -e "\n${BOLD}═══════════════════════════════════════════════════════════════════════════${NC}"
  echo -e "${BOLD}${GREEN}✔ ALL 5 SHOWCASE DEMOS COMPLETED SUCCESSFULLY${NC}"
  echo -e "${BOLD}═══════════════════════════════════════════════════════════════════════════${NC}"
}

if [[ "${1:-}" == "--all" || "${1:-}" == "--batch" || "${1:-}" == "--non-interactive" ]]; then
  run_all "$@"
  exit 0
fi

if [[ "${1:-}" == "--demo" && -n "${2:-}" ]]; then
  run_demo "$2"
  exit 0
fi

if [[ -n "${1:-}" && "${1:-}" =~ ^[1-5]$ ]]; then
  run_demo "$1"
  exit 0
fi

# Interactive menu
print_header
echo -e "Select a showcase demo to execute:\n"
echo -e "  ${BOLD}[1]${NC} Day-0 Greenfield DX     (Instant setup, starter graph, health verification)"
echo -e "  ${BOLD}[2]${NC} Crash-Consistency       (Unannounced kill -9, WAL replay, zero data loss)"
echo -e "  ${BOLD}[3]${NC} Dynamic Membrane        (MCP directory jail, prompt injection trapping)"
echo -e "  ${BOLD}[4]${NC} Cryptographic Audit     (Multi-agent custody, CISA OpenVEX attestation)"
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
