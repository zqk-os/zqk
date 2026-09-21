#!/bin/sh
# Full-tree scan: fail closed on hardcoded canonical CLI invocations ("zqk <verb>").
set -eu

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO_ROOT" || exit 1

exec go run ./scripts/check_cli_name_literals "$REPO_ROOT" "$@"
