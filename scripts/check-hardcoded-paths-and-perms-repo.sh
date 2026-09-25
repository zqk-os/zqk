#!/bin/sh
# Full-tree scan: enforce AST hygiene for path joins, file permissions, and duplicate literals.
set -eu

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO_ROOT" || exit 1

# Filter legacy flags not recognized by zqk-vet
ARGS=""
for arg in "$@"; do
  case "$arg" in
    --no-dups|-no-dups)
      ;;
    *)
      ARGS="$ARGS $arg"
      ;;
  esac
done

if [ -x "./bin/zqk-vet" ]; then
  exec ./bin/zqk-vet --suite hygiene $ARGS
fi

exec go run ./cmd/zqk-vet --suite hygiene $ARGS
