#!/bin/sh
# Remove studio-internal onboarding files and the leftover studio architecture
# dump. First-run surface is sku-overlay last-wins (COMMUNITY_FIRST_RUN,
# QUICKSTART, FIRST_RUN_OBJECT_TUTORIAL, onboarding README, architecture stubs).
# Usage: prune-community-onboarding.sh <dest-root>
# TRACK: TDE-1789690070487265000-ea5471f4
set -eu

DEST=${1:-}
if [ -z "$DEST" ] || [ ! -d "$DEST" ]; then
  echo "usage: $0 <dest-root>" >&2
  exit 2
fi
DEST=$(CDPATH= cd -- "$DEST" && pwd)
HERE=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
ONB="$DEST/docs/onboarding"

# Studio incident notes, mesh/MMORCH dogfood, dated handoffs, and brew/admin
# promote runbooks. Strangers reading these as first-run will 1-star the SKU.
if [ -d "$ONB" ]; then
for name in \
  AGENT_HANDOFF_2026-08-11_POST_REBOOT.md \
  AGENT_HANDOFF_2026-08-12_POST_BASELINE_REBOOT.md \
  AGENT_HANDOFF_2026-08-15_POST_COMMS_REBOOT.md \
  AGENT_HANDOFF_2026-08-28_KERNEL_WIPE.md \
  AGENT_HANDOFF_CYCLE.md \
  AGENT_ONBOARDING_SNAPSHOT.md \
  AGENT_ONBOARDING_SUMMARIES_DIGEST.md \
  AGENT_ONBOARDING_ASSESSMENT_2026-03-21.md \
  STABLE_BINARY_MANAGEMENT.md \
  MMORCH_PEER_SESSION_BOOTSTRAP.md \
  MESH_COMMS_E2E_SETUP.md \
  TEST_MCP_ECHO.md \
  TEST_ECHO_TOOL.md \
  BETA_CURRICULUM_AND_SCREEN_MAP.md \
  RESTORED_PROJECT_CONFIGURATION.md \
  QUICK_CONTEXT_HELPERS.md \
  NEW_AGENT_PROTOCOL.md \
  PROJECT_ROOT_WIPE_HAZARDS.md \
  IMPORT_CYCLE_RESOLUTION.md \
  PACKAGE_CHECKLIST.md \
  CORPORATE_SETUP_GUIDE.md \
  CLI_COMMAND_TAXONOMY.md \
  CLI_Effective_Use.md \
  SYSTEM_OBJECTS_GUIDE.md \
  ZQK_Features.md \
  Persona_Guides.md \
  PBC_CHARTER_STATEMENT.md \
  EFFICIENT_DATA_PROCESSING.md \
  FIELD_STATE_TRACKING_PRINCIPLES.md \
  CURSOR_MCP_SETUP.md \
  MCP_CONFIGURATION.md
do
  rm -f "$ONB/$name"
done
rm -rf "$ONB/archive"
fi
rm -rf "$DEST/docs/launch"

ARCH="$DEST/docs/architecture"
if [ -d "$ARCH" ]; then
  rm -rf "$ARCH"
  mkdir -p "$ARCH"
  if [ -f "$HERE/sku-overlay/ARCHITECTURE_README.md" ]; then
    cp "$HERE/sku-overlay/ARCHITECTURE_README.md" "$ARCH/README.md"
    cp "$HERE/sku-overlay/ARCHITECTURE_INDEX.md" "$ARCH/INDEX.md"
  fi
fi

# Leftover studio Diataxis / coding dump. First-run lives in onboarding.
for dir in \
  "$DEST/docs/best-practices" \
  "$DEST/docs/tutorials" \
  "$DEST/docs/howto" \
  "$DEST/docs/explanation" \
  "$DEST/docs/reference" \
  "$DEST/docs/manual"
do
  rm -rf "$dir"
done
rm -f "$DEST/docs/cli-reference.md"
if [ -f "$HERE/sku-overlay/GETTING_STARTED.md" ]; then
  cp "$HERE/sku-overlay/GETTING_STARTED.md" "$DEST/docs/getting-started.md"
fi
if [ -f "$HERE/sku-overlay/ZQK_GETTING_STARTED.md" ]; then
  cp "$HERE/sku-overlay/ZQK_GETTING_STARTED.md" "$DEST/ZQK_GETTING_STARTED.md"
fi
if [ -f "$HERE/sku-overlay/AGENT_BOOT.md" ]; then
  cp "$HERE/sku-overlay/AGENT_BOOT.md" "$DEST/.iderules"
  cp "$HERE/sku-overlay/AGENT_BOOT.md" "$DEST/.clinerules"
  cp "$HERE/sku-overlay/AGENT_BOOT.md" "$DEST/.windsurfrules"
fi

echo "Pruned studio-internal onboarding from $ONB"
