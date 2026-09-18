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

install_file "$SKU/COMMUNITY_FIRST_RUN.md" "$DEST/docs/onboarding/COMMUNITY_FIRST_RUN.md"
install_file "$SKU/FIRST_RUN_OBJECT_TUTORIAL.md" "$DEST/docs/onboarding/FIRST_RUN_OBJECT_TUTORIAL.md"
install_file "$SKU/QUICKSTART.md" "$DEST/docs/onboarding/QUICKSTART.md"
install_file "$SKU/CONTRIBUTING.md" "$DEST/CONTRIBUTING.md"
install_file "$SKU/README.md" "$DEST/README.md"
echo "Installed community SKU overlay into $DEST"
