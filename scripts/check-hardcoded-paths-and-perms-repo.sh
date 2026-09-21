#!/bin/sh
# Full-tree scan: enforce AST hygiene for path joins, file permissions, and duplicate literals.
set -eu

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO_ROOT" || exit 1

exec go run ./scripts/check_path_and_perm_literals "$REPO_ROOT" "$@"
