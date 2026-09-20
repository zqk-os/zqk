#!/bin/sh
# Apply the shared post-copy source boundary to every community export path.
set -eu

ROOT=${1:-}
if [ -z "$ROOT" ] || [ ! -d "$ROOT" ]; then
	echo "usage: $0 <community-root>" >&2
	exit 2
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)

rm -rf "$ROOT/docs/launch" "$ROOT/pkg/mesh" "$ROOT/pkg/agent"
if [ -d "$ROOT/docs" ]; then
	find "$ROOT/docs" -depth -type d \( -name archive -o -name _archive \) -exec rm -rf {} + 2>/dev/null || true
fi

rm -rf "$ROOT/cmd/zqk-community/pkg_cmd/healthchk"
rm -f "$ROOT/cmd/zqk-community/app/join.go" \
	"$ROOT/pkg/brand/apply.go" \
	"$ROOT/pkg/brand/apply_test.go" \
	"$ROOT/pkg/brand/project_config.go" \
	"$ROOT/pkg/brand/project_config_test.go"

# TRACK: TDE-1789712164445942000-2f9cf61a — include Helm tests only after
# the community distribution has a dest-owned design and public chart assets.
rm -f "$ROOT/pkg/community/container_helm_test.go"

if [ -d "$ROOT/pkg/cli" ]; then
	find "$ROOT/pkg/cli" \( \
		-name '*mesh*_command_builder.go' -o \
		-name '*keystore*_command_builder.go' -o \
		-name '*evolve*_command_builder.go' -o \
		-name '*agent_lockdown*_command_builder.go' -o \
		-name 'ambient_*_command_builder.go' -o \
		-name 'agent_*_command_builder.go' -o \
		-name '*paste*applescript*_command_builder.go' \
	\) -type f -delete 2>/dev/null || true
fi

rm -f "$ROOT/cmd/zqk/system/agent_lockdown.go" \
	"$ROOT/cmd/zqk/system/ambient_daemon.go" \
	"$ROOT/cmd/zqk/system/sync_agents.go" \
	"$ROOT/cmd/zqk/system/evolve.go" \
	"$ROOT/cmd/zqk/system/materialize_agent_chat_channel.go" \
	"$ROOT/cmd/zqk/system/materialize_agent_chat_channel_test.go"

if [ -f "$ROOT/cmd/zqk/system/register_studio_commands.go" ]; then
	cat >"$ROOT/cmd/zqk/system/register_studio_commands.go" <<'EOF'
package system

import "github.com/spf13/cobra"

// registerStudioCommands is a no-op in the open-core community edition.
func registerStudioCommands(_ *cobra.Command) {}
EOF
fi

if [ -f "$ROOT/.gitignore" ]; then
	grep -Ev '^\*-\*\.txt$|^\*test\*\.txt$|^\*_results\.txt$' "$ROOT/.gitignore" >"$ROOT/.gitignore.tmp"
	mv "$ROOT/.gitignore.tmp" "$ROOT/.gitignore"
	grep -Fqx '.zcom/' "$ROOT/.gitignore" || printf '%s\n' '.zcom/' >>"$ROOT/.gitignore"
	grep -Fqx 'config/zqk-local.yaml' "$ROOT/.gitignore" || printf '%s\n' 'config/zqk-local.yaml' >>"$ROOT/.gitignore"
fi
