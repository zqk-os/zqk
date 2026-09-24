#!/usr/bin/env bash
# ZQK Killer Demo 5: Interactive Object Inspector, Live Policy Studio & Unified QA
# Demonstrates:
#   1. Human-friendly Interactive Object Inspector (zqk object inspect)
#   2. Dual Semantic Agent JSON Projections (-f json) with reduced token footprint
#   3. Modular Display Cards (Lineage Traceability Radar, CAS Storage & Ontology Profiles)
#   4. Deep Drill-Down Record Inspection with Role-Gated Action Palette
#   5. Live Policy Rule Studio with real-time DSL evaluation & autocompletion
#   6. Unified Mission Control QA Tab & Test Matrix Lineage Traceability
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
ZQK_BIN="${ROOT_DIR}/bin/zqk"

if [[ ! -x "${ZQK_BIN}" ]]; then
  echo "Error: zqk binary not found at ${ZQK_BIN}. Run 'go build -o ./bin/zqk ./cmd/zqk' first." >&2
  exit 1
fi

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

clear 2>/dev/null || true
echo -e "${BOLD}═══════════════════════════════════════════════════════════════════════════${NC}"
echo -e "${BOLD}${CYAN}   ZQK DEMO 5: OBJECT INSPECTOR, POLICY STUDIO & UNIFIED QA CONSOLE       ${NC}"
echo -e "${BOLD}═══════════════════════════════════════════════════════════════════════════${NC}"
echo -e "  ${YELLOW}Thesis:${NC} Managing complex knowledge kernel graphs should not require"
echo -e "  memorizing dozens of CLI flags or parsing walls of raw YAML."
echo -e "  Humans get clean, interactive drill-downs and policy studios;"
echo -e "  autonomous AI agents receive dense, token-optimized semantic projections."
echo -e "${BOLD}═══════════════════════════════════════════════════════════════════════════${NC}\n"

sleep 1

echo -e "${BOLD}[SCENE 1: DUAL HUMAN / AGENT SEMANTIC PROJECTIONS]${NC}"
echo -e "  Autonomous agents need high signal, low token overhead."
echo -e "  Querying single object with agent-reduced semantic projection (-f json):\n"
echo -e "  $ ${BOLD}zqk object inspect backlog_item BLI-OBJECT-INSPECT-POL-001 -f json${NC}\n"
sleep 0.8

"${ZQK_BIN}" object inspect backlog_item BLI-OBJECT-INSPECT-POL-001 -f json | head -n 35
echo -e "${DIM}  ... [stream continues with storage profile, ontology & lineage]${NC}\n"

sleep 1.2
echo -e "${BOLD}[SCENE 2: HUMAN INTERACTIVE OBJECT INSPECTOR (TDS FORMAT)]${NC}"
echo -e "  Human operators get structured TDS panel views with Modular Cards:\n"
echo -e "  $ ${BOLD}zqk object inspect backlog_item BLI-OBJECT-INSPECT-POL-001${NC}\n"
sleep 0.8

"${ZQK_BIN}" object inspect backlog_item BLI-OBJECT-INSPECT-POL-001

sleep 1.2
echo -e "\n${BOLD}[SCENE 3: LIVE POLICY RULE STUDIO & DSL DRY-RUN ENGINE]${NC}"
echo -e "  Validating active backlog items against kernel policy rules in real time:\n"
echo -e "  $ ${BOLD}zqk object inspect backlog_item --policy-studio -f json${NC}\n"
sleep 0.8

"${ZQK_BIN}" object inspect backlog_item --policy-studio -f json | head -n 30
echo -e "${DIM}  ... [evaluated rules against repository objects in dry-run mode]${NC}\n"

sleep 1.2
echo -e "${BOLD}[SCENE 4: UNIFIED MISSION CONTROL QA TAB & TEST TRACEABILITY]${NC}"
echo -e "  Verifying Definition of Done (DoD) gate across all test suites and criteria:\n"
echo -e "  $ ${BOLD}zqk test dashboard --check-dod${NC}\n"
sleep 0.8

"${ZQK_BIN}" test dashboard --check-dod

sleep 1
echo -e "\n${BOLD}[SCENE 5: INTERACTIVE TUI SHORTCUT SUMMARY]${NC}"
echo -e "  Operators can launch full-screen interactive TUI modes with keyboard shortcuts:"
echo -e "  • ${CYAN}zqk object inspect [kind]${NC}   Full TUI with Vim navigation (j/k/g/G), search (/),"
echo -e "                               sort cycling ([s]), filter pills ([f]), drill-down ([Enter]),"
echo -e "                               action palette ([a]), and policy studio ([p])."
echo -e "  • ${CYAN}zqk ui --tab qa${NC}             Mission Control QA Tab with live test suites table,"
echo -e "                               intact lineage chains, unbound criteria alerts, and [t] re-scan."
echo -e "  • ${CYAN}[z / ?]${NC}                    Instant toggle between Newb, Pro, and Jedi (Zen) modes."

echo -e "\n${BOLD}═══════════════════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}${BOLD}✓ DEMO 5 COMPLETE: Full Human & Machine Inspection Parity Achieved!${NC}"
echo -e "${BOLD}═══════════════════════════════════════════════════════════════════════════${NC}\n"
