#!/bin/sh
# Verify embedded bootstrap extracts into a *temporary* project root.
# The source repo must never be used as the user project root.
#
# Usage: verify-bootstrap-portable.sh <repo-root> [community-binary]
set -eu
REPO_ROOT=${1:-}
BIN_HINT=${2:-}
if [ -z "$REPO_ROOT" ] || [ ! -d "$REPO_ROOT" ]; then
  echo "usage: $0 <repo-root> [community-binary]" >&2
  exit 2
fi
REPO_ROOT=$(CDPATH= cd -- "$REPO_ROOT" && pwd)

# Package-level extract proof (does not create .zqk in the repo).
# Isolated TEST_ROOT satisfies pkg/zqkenv agent_guard (and keeps extract off the studio tree).
cd "$REPO_ROOT"
TEST_ROOT=$(mktemp -d "${TMPDIR:-/tmp}/zqk-bootstrap-verify.XXXXXX")
trap 'rm -rf "$TEST_ROOT"' EXIT INT TERM
export ZQK_TEST_ROOT="$TEST_ROOT"
go test ./pkg/bootstrap -count=1 -timeout 60s \
  -run 'TestManifestPaths_EmbeddedArchive|TestExtractEmbeddedToTempProject'

# Optional: binary exists and is not "initialized" against the repo.
if [ -n "$BIN_HINT" ] && [ -x "$REPO_ROOT/$BIN_HINT" ]; then
  if [ -d "$REPO_ROOT/.zqk" ] && [ -f "$REPO_ROOT/.zqk/agent-runtime/agent_workspace_sync.json" ]; then
    echo "WARN: repo looks like an initialized ZQK project (.zqk/agent-runtime)." >&2
    echo "      Community source trees should stay uninitialized; use a separate project dir." >&2
  fi
fi

echo "✓ Bootstrap portable verify OK (extract → temp project only; repo ≠ project root)"
