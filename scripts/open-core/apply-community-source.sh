#!/bin/bash
# Apply community source overlay non-destructively to a target checkout (e.g. zqk-public-candidate).
# Crucially: NEVER touches or clobbers .zqk/process, .zqk/state, .zqk-state, or .env.
# TRACK: TDE-1789678536875854000-47240146 (PRI-1789678738709181000-3832006d)

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
  ".goreleaser.yaml"
  ".gitignore"
  "NOTICE"
  "go.mod"
  "go.sum"
  "LICENSE"
  "README.md"
)

# Safety check: Guard .zqk/process and .env before copying
if [ -d "$DEST/.zqk/process" ]; then
  echo "✓ Guarding existing $DEST/.zqk/process (preserved untouched)"
fi

for path in "${OVERLAY_PATHS[@]}"; do
  # Studio Makefile must never land on dest. Community SKU installs
  # Makefile.community as dest/Makefile after this loop.
  # TRACK: TDE-1789681135032251000-e52ad7a7
  case "$path" in
    Makefile) echo "skip $path (community Makefile.community is the dest SKU)"; continue ;;
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

# Prune archived and launch doc trees per open-core governance
rm -rf "$DEST/docs/launch"
if [ -d "$DEST/docs" ]; then
  find "$DEST/docs" -depth -type d \( -name "archive" -o -name "_archive" \) -exec rm -rf {} + 2>/dev/null || true
fi

# Prune excluded subpackages and builders per police-community-tree.sh
rm -rf "$DEST/pkg/mesh"
rm -rf "$DEST/pkg/agent"

if [ -d "$DEST/pkg/cli" ]; then
  find "$DEST/pkg/cli" \( \
    -name '*mesh*_command_builder.go' -o \
    -name '*keystore*_command_builder.go' -o \
    -name '*evolve*_command_builder.go' -o \
    -name '*agent_lockdown*_command_builder.go' -o \
    -name 'ambient_*_command_builder.go' -o \
    -name 'agent_*_command_builder.go' -o \
    -name '*paste*applescript*_command_builder.go' \
  \) -exec rm -f {} + 2>/dev/null || true
fi

# Prune studio-specific system commands per police-community-tree.sh
rm -f "$DEST/cmd/zqk/system/agent_lockdown.go" \
      "$DEST/cmd/zqk/system/ambient_daemon.go" \
      "$DEST/cmd/zqk/system/sync_agents.go" \
      "$DEST/cmd/zqk/system/evolve.go" \
      "$DEST/cmd/zqk/system/materialize_agent_chat_channel.go"

# Stub studio command registration in candidate tree
if [ -f "$DEST/cmd/zqk/system/register_studio_commands.go" ]; then
  cat << 'EOF' > "$DEST/cmd/zqk/system/register_studio_commands.go"
package system

import "github.com/spf13/cobra"

// registerStudioCommands is a no-op in the open-core community edition.
func registerStudioCommands(_ *cobra.Command) {}
EOF
fi

# Clean overly broad patterns from community .gitignore
if [ -f "$DEST/.gitignore" ]; then
  grep -Ev '^\*-\*\.txt$|^\*test\*\.txt$|^\*_results\.txt$' "$DEST/.gitignore" > "$DEST/.gitignore.tmp" && mv "$DEST/.gitignore.tmp" "$DEST/.gitignore"
fi

echo "✓ Community source overlay complete at $DEST"
HERE="$(cd "$(dirname "$0")" && pwd)"
if [ -x "$HERE/install-community-makefile.sh" ]; then
  sh "$HERE/install-community-makefile.sh" "$DEST"
fi
