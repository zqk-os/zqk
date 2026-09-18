#!/bin/sh
# ZQK Community pressure-test installer. Builds ./bin/zcom from this checkout.
# There is no public brew formula or GitHub release yet. Do not clone lanceman/zqk.
# TRACK: TDE-1789681135032251000-e52ad7a7
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
INSTALL_DIR="${ZCOM_INSTALL_DIR:-${ZQK_INSTALL_DIR:-$ROOT/bin}}"

if [ ! -f "$ROOT/cmd/zqk-community/main.go" ]; then
  echo "This installer only works from the community product tree (cmd/zqk-community)." >&2
  exit 2
fi

echo "Building zcom from $ROOT"
make -C "$ROOT" zcom

mkdir -p "$INSTALL_DIR"
install -m 755 "$ROOT/bin/zcom" "$INSTALL_DIR/zcom"

echo ""
echo "Installed $INSTALL_DIR/zcom"
echo "Quick start:"
echo "  cd <project>"
echo "  $INSTALL_DIR/zcom system init --project-name <name>"
echo "  $INSTALL_DIR/zcom quickstart"
echo ""
echo "Do not export ZCOM_PROJECT_ROOT. Docs: docs/onboarding/COMMUNITY_FIRST_RUN.md"
