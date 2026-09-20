#!/bin/sh
# Install the community Makefile over dest/Makefile.
# Studio keeps its full Makefile; this SKU template is renamed only in exports.
set -eu

DEST=${1:-}
if [ -z "$DEST" ] || [ ! -d "$DEST" ]; then
  echo "usage: $0 <dest-root>" >&2
  exit 2
fi
DEST=$(CDPATH= cd -- "$DEST" && pwd)
HERE=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
SRC="$HERE/Makefile.community"
if [ ! -f "$SRC" ]; then
  echo "REFUSE: missing $SRC" >&2
  exit 2
fi
cp "$SRC" "$DEST/Makefile"
echo "Installed community Makefile from $SRC"
