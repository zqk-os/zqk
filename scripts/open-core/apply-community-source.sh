#!/bin/bash
# Apply community source overlay non-destructively to a target checkout (e.g. zqk-public-candidate).
# Crucially: NEVER touches or clobbers .zqk/process, .zqk/state, .zqk-state, or .env.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"

DEST="${1:-}"
if [ -z "$DEST" ]; then
  echo "Usage: $0 <destination-directory>" >&2
  exit 1
fi

if [ ! -d "$DEST" ]; then
  echo "Error: destination directory $DEST does not exist." >&2
  exit 1
fi

DEST="$(cd "$DEST" && pwd)"
if [ "$DEST" = "$REPO_ROOT" ]; then
  echo "Error: destination cannot be the source studio repo root ($REPO_ROOT)." >&2
  exit 1
fi

echo "Overlaying community source from $REPO_ROOT to $DEST"

# Explicit paths to overlay
OVERLAY_PATHS=(
  "cmd/zqk-community"
  "cmd/zqk-shim"
  "cmd/zqk/docman"
  "cmd/zqk/grep"
  "cmd/zqk/healthchk"
  "cmd/zqk/inbox"
  "cmd/zqk/learn"
  "cmd/zqk/mcp"
  "cmd/zqk/new"
  "cmd/zqk/object"
  "cmd/zqk/system"
  "cmd/zqk/tray"
  "cmd/zqk/utility"
  "cmd/zqk/workflow"
  "pkg"
  "internal"
  "ext"
  "docs/architecture"
  "docs/best-practices"
  "docs/onboarding"
  "docs/tutorials"
  "docs/howto"
  "docs/reference"
  "docs/explanation"
  "docs/INDEX.md"
  "docs/getting-started.md"
  "docs/cli-reference.md"
  "CONTRIBUTING.md"
  "CODE_OF_CONDUCT.md"
  "scripts/build-bootstrap-archive.sh"
  "scripts/starter_kernel_graph"
  "scripts/open-core"
  "scripts/package-community.sh"
  "scripts/install.sh"
  "scripts/default_policies"
  "scripts/default_personas"
  "scripts/default_agent_skills"
  ".zqk/specs"
  ".zqk/cli/specs"
  ".zqk/cli/command_spec_coverage_baseline.json"
  ".gitignore"
  "NOTICE"
  "go.mod"
  "go.sum"
  "LICENSE"
)

# Safety check: Guard .zqk/process and .env before copying
if [ -d "$DEST/.zqk/process" ]; then
  echo "✓ Guarding existing $DEST/.zqk/process (preserved untouched)"
fi

for path in "${OVERLAY_PATHS[@]}"; do
  # Studio first-run copy must never replace the community-owned release files.
  case "$path" in
    Makefile|README.md|CONTRIBUTING.md)
      echo "skip $path (community SKU overlay reinstalls dest-owned first-run files)"
      continue
      ;;
  esac
  src="$REPO_ROOT/$path"
  dst="$DEST/$path"
  if [ -e "$src" ]; then
    mkdir -p "$(dirname "$dst")"
    if [ -d "$src" ]; then
      mkdir -p "$dst"
      # Copy directory contents overlaying files without wiping destination
      cp -R "$src/"* "$dst/" 2>/dev/null || cp -R "$src" "$(dirname "$dst")"
    else
      cp -f "$src" "$dst"
    fi
  fi
done

sh "$DEST/scripts/open-core/rewrite-community-module-path.sh" "$DEST"
sh "$DEST/scripts/open-core/prune-community-source-boundary.sh" "$DEST"

# Prune studio-internal onboarding files and docs noise
if [ -x "$REPO_ROOT/scripts/open-core/prune-community-onboarding.sh" ]; then
  "$REPO_ROOT/scripts/open-core/prune-community-onboarding.sh" "$DEST"
fi

# Always install community SKU overlay (Makefile, README, first-run docs)
if [ -x "$REPO_ROOT/scripts/open-core/install-community-sku.sh" ]; then
  "$REPO_ROOT/scripts/open-core/install-community-sku.sh" "$DEST"
else
  "$REPO_ROOT/scripts/open-core/install-community-makefile.sh" "$DEST"
fi

echo "✓ Community source overlay complete at $DEST"
