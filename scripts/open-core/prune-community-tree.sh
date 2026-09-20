#!/bin/sh
# Prune studio-only trees from a community-bounded checkout.
# Safe for seated product: never touches .zqk/process.
# Usage: prune-community-tree.sh <dest-root>
set -eu

DEST=${1:-}
if [ -z "$DEST" ] || [ ! -d "$DEST" ]; then
  echo "usage: $0 <dest-root>" >&2
  exit 2
fi
DEST=$(CDPATH= cd -- "$DEST" && pwd)

# TRACK: TDE-1789629838711755000-bc3ed5d4 — do not ship launch / archive docs.
rm -rf "$DEST/docs/launch"
if [ -d "$DEST/docs" ]; then
  find "$DEST/docs" -depth -type d \( -name "archive" -o -name "_archive" \) -exec rm -rf {} + 2>/dev/null || true
fi

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
    \) -type f -delete 2>/dev/null || true
fi

rm -rf "$DEST/cmd/zqk-community/pkg_cmd/healthchk"

rm -f "$DEST/cmd/zqk/system/agent_lockdown.go" \
      "$DEST/cmd/zqk/system/ambient_daemon.go" \
      "$DEST/cmd/zqk/system/sync_agents.go" \
      "$DEST/cmd/zqk/system/evolve.go" \
      "$DEST/cmd/zqk/system/materialize_agent_chat_channel.go"

# Studio main is MUST_NOT on community trees (police-community-tree.sh).
rm -rf "$DEST/cmd/zqk/main.go" "$DEST/cmd/zqk/app"

if [ -f "$DEST/cmd/zqk/system/register_studio_commands.go" ]; then
  cat << 'EOF' > "$DEST/cmd/zqk/system/register_studio_commands.go"
package system

import "github.com/spf13/cobra"

// registerStudioCommands is a no-op in the open-core community edition.
func registerStudioCommands(_ *cobra.Command) {}
EOF
fi

if [ -f "$DEST/.gitignore" ]; then
  grep -Ev '^\*-\*\.txt$|^\*test\*\.txt$|^\*_results\.txt$' "$DEST/.gitignore" > "$DEST/.gitignore.tmp" && mv "$DEST/.gitignore.tmp" "$DEST/.gitignore"
fi
