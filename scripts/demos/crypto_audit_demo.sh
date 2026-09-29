#!/usr/bin/env bash
# ZQK Demonstration 4: The Verifiable Cryptographic Ledger & CISA OpenVEX Attestation
# Demonstrates: Multi-agent cryptographic seating, SHA-256 fingerprinting, vault keystore, and CISA OpenVEX attestation.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
ZQK_BIN="${ROOT_DIR}/bin/zqk"
GEN_OPENVEX="${ROOT_DIR}/scripts/generate-openvex.sh"

if [[ ! -x "${ZQK_BIN}" ]]; then
  echo "Error: zqk binary not found at ${ZQK_BIN}. Run 'go build -o ./bin/zqk ./cmd/zqk' first." >&2
  exit 1
fi

if [[ ! -x "${GEN_OPENVEX}" ]]; then
  chmod +x "${GEN_OPENVEX}" 2>/dev/null || true
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
echo -e "${BOLD}===========================================================================${NC}"
echo -e "${BOLD}${CYAN}  ZQK DEMO 4: THE VERIFIABLE CRYPTOGRAPHIC LEDGER & OPENVEX ATTESTATION    ${NC}"
echo -e "${BOLD}===========================================================================${NC}"
echo -e "  ${YELLOW}Thesis:${NC} Anyone can generate code. ZQK generates cryptographically"
echo -e "  verifiable proof of how the code was made. You don't have to trust the LLM;"
echo -e "  you can mathematically verify the exact constraint and seating path taken."
echo -e "${BOLD}===========================================================================${NC}\n"

sleep 0.5

TMP_PROJECT="$(mktemp -d /tmp/zqk-crypto-demo-XXXXXX)"
cleanup() {
  rm -rf "${TMP_PROJECT}" 2>/dev/null || true
}
trap cleanup EXIT

echo -e "${BOLD}[SCENE 1: INITIALIZING CRYPTOGRAPHIC VAULT WORKSPACE]${NC}"
(cd "${TMP_PROJECT}" && "${ZQK_BIN}" system init --project-name "verifiable-ledger-core")

echo -e "\n${BOLD}[SCENE 2: MULTI-AGENT CRYPTOGRAPHIC SEAT ISSUANCE]${NC}"
echo -e "  Issuing unique cryptographic API key and seating file for Architect agent:"
echo -e "  $ ${BOLD}zqk keystore issue --account-id ACC-ARCH-01 --title \"Swarm-Architect\"${NC}\n"

(cd "${TMP_PROJECT}" && "${ZQK_BIN}" keystore issue --account-id ACC-ARCH-01 --title "Swarm-Architect")

echo -e "\n  Issuing unique cryptographic API key and seating file for Security Reviewer agent:"
echo -e "  $ ${BOLD}zqk keystore issue --account-id ACC-REV-02 --title \"Swarm-Security-Reviewer\"${NC}\n"

(cd "${TMP_PROJECT}" && "${ZQK_BIN}" keystore issue --account-id ACC-REV-02 --title "Swarm-Security-Reviewer")

sleep 0.8

echo -e "\n${BOLD}[SCENE 3: INSPECTING LOCAL VAULT KEYSTORE LEDGER]${NC}"
echo -e "  Querying active keystore entries and verified agent identities:"
echo -e "  $ ${BOLD}zqk keystore list${NC}\n"

(cd "${TMP_PROJECT}" && "${ZQK_BIN}" keystore list)

sleep 0.8

echo -e "\n${BOLD}[SCENE 4: GENERATING CISA OPENVEX ATTESTATION & SBOM]${NC}"
echo -e "  Minting CISA OpenVEX v0.2.0 attestation for product release:"
echo -e "  $ ${BOLD}./scripts/generate-openvex.sh v0.1.0 ${TMP_PROJECT}/openvex.json${NC}\n"

VEX_FILE="${TMP_PROJECT}/openvex.json"
"${GEN_OPENVEX}" "v0.1.0" "${VEX_FILE}"

echo -e "\n  Attestation preview (first 25 lines of OpenVEX JSON):"
head -n 25 "${VEX_FILE}"
echo -e "${DIM}  ... [complete OpenVEX document with signed vulnerability statements]${NC}"

sleep 0.8

echo -e "\n${BOLD}[SCENE 5: INDEPENDENT CRYPTOGRAPHIC LEDGER VERIFICATION]${NC}"
echo -e "  Validating OpenVEX schema conformance and vulnerability status:"
echo -e "  $ ${BOLD}./scripts/generate-openvex.sh --verify ${TMP_PROJECT}/openvex.json${NC}\n"

"${GEN_OPENVEX}" --verify "${VEX_FILE}"

echo -e "\n${BOLD}===========================================================================${NC}"
echo -e "${GREEN}${BOLD}✔ DEMO 4 COMPLETE: Multi-Agent Ledger & Attestation Cryptographically Verified!${NC}"
echo -e "  ├─ Multi-Agent Seating: Real SHA-256 fingerprints & tokens issued per seat"
echo -e "  ├─ Vault Keystore: Stored in isolated cellular membrane, credentials protected"
echo -e "  ├─ CISA OpenVEX v0.2.0: Full cryptographic attestation generated"
echo -e "  └─ Independent Verification: 100% compliant tamper-evident audit trail"
echo -e "${BOLD}===========================================================================${NC}\n"
