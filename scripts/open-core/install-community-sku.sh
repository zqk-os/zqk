#!/bin/sh
# Re-install dest-owned SKU files after a studio overlay so first-run docs/Makefile
# cannot be replaced by brew/zqk/scheduler copy-paste.
# Usage: install-community-sku.sh <dest-root>
# TRACK: TDE-1789690070487265000-ea5471f4
set -eu

DEST=${1:-}
if [ -z "$DEST" ] || [ ! -d "$DEST" ]; then
  echo "usage: $0 <dest-root>" >&2
  exit 2
fi
DEST=$(CDPATH= cd -- "$DEST" && pwd)
HERE=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
SOURCE_ROOT=$(CDPATH= cd -- "$HERE/../.." && pwd)

if [ -x "$HERE/install-community-makefile.sh" ]; then
  sh "$HERE/install-community-makefile.sh" "$DEST"
fi

SKU="$HERE/sku-overlay"
if [ ! -d "$SKU" ]; then
  echo "skip sku-overlay (missing $SKU)"
  exit 0
fi

install_file() {
  src=$1
  dst=$2
  if [ -f "$src" ]; then
    mkdir -p "$(dirname "$dst")"
    cp "$src" "$dst"
    echo "sku-overlay: $(basename "$dst")"
  fi
}

install_apache_license() {
  src=$1
  dst=$2
  if [ -f "$src" ]; then
    awk '/^--------------------------------------------------------------------------------$/{exit} {print}' "$src" >"$dst"
    echo "sku-overlay: $(basename "$dst")"
  fi
}

install_file "$SKU/AI_AGENT_ONBOARDING.md" "$DEST/docs/onboarding/AI_AGENT_ONBOARDING.md"
install_file "$SKU/COMMUNITY_FIRST_RUN.md" "$DEST/docs/onboarding/COMMUNITY_FIRST_RUN.md"
install_file "$SKU/FIRST_RUN_OBJECT_TUTORIAL.md" "$DEST/docs/onboarding/FIRST_RUN_OBJECT_TUTORIAL.md"
install_file "$SKU/QUICKSTART.md" "$DEST/docs/onboarding/QUICKSTART.md"
install_file "$SKU/ONBOARDING_README.md" "$DEST/docs/onboarding/README.md"
install_file "$SKU/EDGE_HEADLESS_FIRST_RUN.md" "$DEST/docs/onboarding/EDGE_HEADLESS_FIRST_RUN.md"
install_file "$SKU/CONTRIBUTING.md" "$DEST/CONTRIBUTING.md"
install_apache_license "$SOURCE_ROOT/LICENSE" "$DEST/LICENSE"
install_file "$SKU/NOTICE" "$DEST/NOTICE"
install_file "$SKU/SECURITY.md" "$DEST/SECURITY.md"
install_file "$SKU/CI.yml" "$DEST/.github/workflows/ci.yml"
install_file "$SKU/README.md" "$DEST/README.md"
install_file "$SKU/ARCHITECTURE_README.md" "$DEST/docs/architecture/README.md"
install_file "$SKU/ARCHITECTURE_INDEX.md" "$DEST/docs/architecture/INDEX.md"
install_file "$SKU/GETTING_STARTED.md" "$DEST/docs/getting-started.md"
install_file "$SKU/ZQK_GETTING_STARTED.md" "$DEST/ZQK_GETTING_STARTED.md"
install_file "$SKU/AGENT_BOOT.md" "$DEST/.iderules"
install_file "$SKU/AGENT_BOOT.md" "$DEST/.clinerules"
install_file "$SKU/AGENT_BOOT.md" "$DEST/.windsurfrules"
echo "Installed community SKU overlay into $DEST"
# applybrand is local-only (reads config/zqk-local.yaml). Do not run it from
# install-community-sku.sh or dest first-run docs get rewritten to a pressure-test name.
