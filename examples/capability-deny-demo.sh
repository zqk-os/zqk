#!/usr/bin/env sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
BIN=${ZQK_BIN:-"$ROOT/bin/zqk"}
if [ ! -x "$BIN" ]; then
  BIN=$(command -v zqk || true)
fi
if [ -z "${BIN:-}" ] || [ ! -x "$BIN" ]; then
  echo "zqk binary not found; build with: go build -o bin/zqk ./cmd/zqk" >&2
  exit 1
fi
echo "== capability list =="
"$BIN" capability list --format json
echo "== forbidden action (expect typed denial) =="
if "$BIN" capability authorize --name demo:forbidden --agent-id stranger --format json; then
  echo "expected denial for demo:forbidden" >&2
  exit 1
fi
echo "== recover on allowed path =="
"$BIN" capability authorize --name demo:safe --agent-id stranger --format json
echo "demo ok: forbidden blocked; allowed recovered"
