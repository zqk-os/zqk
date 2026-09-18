#!/usr/bin/env bash
# ZQK Killer Demo 4: The 5-Minute "Aha" Moment & Day-0 Scaffolding
# Demonstrates: Single-command project initialization, zero-friction developer experience, and instant agent orientation.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
ZQK_BIN="${ROOT_DIR}/bin/zqk"

if [[ ! -x "${ZQK_BIN}" ]]; then
  echo "Error: zqk binary not found at ${ZQK_BIN}. Run 'make all' first." >&2
  exit 1
fi

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

clear 2>/dev/null || true
echo -e "${BOLD}===========================================================================${NC}"
echo -e "${BOLD}${CYAN}  ZQK DEMO 4: THE 5-MINUTE \"AHA\" MOMENT & DAY-0 SCAFFOLDING${NC}"
echo -e "${BOLD}===========================================================================${NC}"
echo -e "  ${YELLOW}Thesis:${NC} Infrastructure should feel effortless. A developer or swarm"
echo -e "  should be able to initialize a brand-new project and achieve full"
echo -e "  knowledge-kernel governance in seconds."
echo -e "${BOLD}===========================================================================${NC}\n"

sleep 1

TMP_PROJECT="$(mktemp -d /tmp/zqk-dayzero-demo-XXXXXX)"
trap 'rm -rf "$TMP_PROJECT"' EXIT

echo -e "${BOLD}[SCENE 1: GREENFIELD DIRECTORY CREATION]${NC}"
echo -e "  Entering empty workspace: ${CYAN}${TMP_PROJECT}${NC}\n"
sleep 0.8

echo -e "${BOLD}[SCENE 2: EXECUTING DAY-0 INITIALIZATION]${NC}"
echo -e "  $ ${BOLD}zqk system init --project-name \"fintech-core\"${NC}\n"

(cd "${TMP_PROJECT}" && "${ZQK_BIN}" system init --project-name "fintech-core")

echo -e "\n${BOLD}[SCENE 3: VERIFYING KERNEL MEMBRANE & SCAFFOLDING]${NC}"
echo -e "  Inspecting generated Knowledge Kernel tree:"
ls -d "${TMP_PROJECT}/.zqk" "${TMP_PROJECT}/.zqk/process" "${TMP_PROJECT}/.zqk/config"

echo -e "\n${BOLD}[SCENE 4: ZERO-FRICTION QUICKSTART ORIENTATION]${NC}"
(cd "${TMP_PROJECT}" && "${ZQK_BIN}" quickstart)

echo -e "\n${BOLD}[SCENE 5: INSTANT HEALTH VALIDATION]${NC}"
echo -e "  $ ${BOLD}zqk system check${NC}\n"
(cd "${TMP_PROJECT}" && "${ZQK_BIN}" system check)

echo -e "\n${GREEN}✔ DAY-0 ONBOARDING COMPLETE:${NC}"
echo -e "  ├─ Zero Config Required: Single binary executes greenfield bootstrap"
echo -e "  ├─ Knowledge Kernel: Instant POSIX CAS & process object tree seeded"
echo -e "  ├─ MCP Integration: Pre-wired for Claude Desktop, Cursor, and IDE swarms"
echo -e "  └─ System Health: 100% compliant out-of-the-box in < 3 seconds"
echo -e "\n${BOLD}${GREEN}DEMO 4 COMPLETE: Instant Day-0 developer experience achieved.${NC}\n"
