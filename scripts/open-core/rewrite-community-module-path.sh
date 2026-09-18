#!/bin/sh
# Rewrite a bounded community export from Studio's private source module to the
# public module identity. This script refuses a tree that still contains Studio.
# TRACK: BLI-1789702449225534000-eca4a6bd
set -eu

DEST=${1:-}
if [ -z "$DEST" ] || [ ! -d "$DEST" ]; then
	echo "usage: $0 <community-export-root>" >&2
	exit 2
fi
DEST=$(CDPATH= cd -- "$DEST" && pwd)

if [ -e "$DEST/cmd/zqk-admin" ]; then
	echo "REFUSE: destination still contains cmd/zqk-admin; not a bounded community export" >&2
	exit 2
fi

python3 - "$DEST" <<'PY'
import os
import pathlib
import sys

root = pathlib.Path(sys.argv[1])
old = "github.com/lanceman/zqk"
new = "github.com/zqk-os/zqk"
skip_dirs = {".git", "vendor"}

for dirpath, dirnames, filenames in os.walk(root):
    current = pathlib.Path(dirpath)
    dirnames[:] = [
        name
        for name in dirnames
        if name not in skip_dirs
        and current.joinpath(name) != root.joinpath(".zqk", "process")
    ]
    for name in filenames:
        path = current / name
        if name != "go.mod" and path.suffix != ".go":
            continue
        data = path.read_text(encoding="utf-8")
        rewritten = data.replace(old, new)
        if rewritten != data:
            path.write_text(rewritten, encoding="utf-8")
PY

if ! grep -Fqx 'module github.com/zqk-os/zqk' "$DEST/go.mod"; then
	echo "community module rewrite did not produce github.com/zqk-os/zqk" >&2
	exit 1
fi
