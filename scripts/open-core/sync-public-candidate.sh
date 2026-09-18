#!/bin/bash
# Sync a disposable open-core export. Not the community TPM checkout.
# Default dest: <studio-parent>/zqk-public-candidate-export
# Product checkout: <studio-parent>/zqk-public-candidate (git-tracked; never default)
# TRACK: BLI-1789670600000000000-00000001 (PRI-OPENCORE-EXPORT-GATE-001)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
PARENT="$(cd "$REPO_ROOT/.." && pwd)"
SIBLING="$PARENT/zqk-public-candidate"
DEFAULT_DEST="$PARENT/zqk-public-candidate-export"
CANDIDATE_DIR="${ZQK_PUBLIC_CANDIDATE_DIR:-$DEFAULT_DEST}"

# TRACK: BLI-1789619419231762000-7f87694b — never rm -rf a seated community kernel.
seated_kernel() {
  [ -d "$1/.zqk/process" ] || [ -f "$1/.env" ]
}

if [ -d "$CANDIDATE_DIR" ]; then
  CANDIDATE_DIR="$(cd "$CANDIDATE_DIR" && pwd)"
fi
# Refuse by directory name, not "parent of this script". A worktree's parent is
# not ~/zqk-restore-clone, so path equality against SIBLING is not enough.
if [ "$(basename "$CANDIDATE_DIR")" = "zqk-public-candidate" ] && [ "${ZQK_PUBLIC_CANDIDATE_ALLOW_CLOBBER:-}" != "1" ]; then
  echo "REFUSE: dest basename zqk-public-candidate is the community product checkout." >&2
  echo "Studio export is $DEFAULT_DEST. Human break-glass: ZQK_PUBLIC_CANDIDATE_ALLOW_CLOBBER=1" >&2
  exit 2
fi
if seated_kernel "$CANDIDATE_DIR"; then
  echo "REFUSE: $CANDIDATE_DIR looks like a seated kernel (.zqk/process or .env)." >&2
  echo "Default dest is $DEFAULT_DEST. Human break-glass: ZQK_PUBLIC_CANDIDATE_ALLOW_CLOBBER=1" >&2
  exit 2
fi
if [ "$CANDIDATE_DIR" = "$SIBLING" ] && [ "${ZQK_PUBLIC_CANDIDATE_ALLOW_CLOBBER:-}" != "1" ]; then
  echo "REFUSE: refusing to rm -rf well-known sibling $SIBLING" >&2
  echo "Export to $DEFAULT_DEST or set ZQK_PUBLIC_CANDIDATE_DIR to a scratch path." >&2
  exit 2
fi

echo "Syncing public candidate from $REPO_ROOT to $CANDIDATE_DIR"

chmod -R u+w "$CANDIDATE_DIR" 2>/dev/null || true
rm -rf "$CANDIDATE_DIR"
mkdir -p "$CANDIDATE_DIR"

# Paths to strictly INCLUDE
INCLUDES=(
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
  "scripts/package-community.sh"
  "scripts/install.sh"
  "scripts/build-bootstrap-archive.sh"
  "scripts/starter_kernel_graph"
  "scripts/open-core"
  "scripts/default_policies"
  "scripts/default_personas"
  "scripts/default_agent_skills"
  ".zqk/specs"
  ".zqk/cli/specs"
  ".zqk/cli/command_spec_coverage_baseline.community.json"
  ".github/workflows/ci.yml"
  ".gitignore"
  "NOTICE"
  "SECURITY.md"
  "Makefile"
  "go.mod"
  "go.sum"
  "LICENSE"
  "README.md"
)

for path in "${INCLUDES[@]}"; do
  if [ -e "$REPO_ROOT/$path" ]; then
    mkdir -p "$(dirname "$CANDIDATE_DIR/$path")"
    cp -R "$REPO_ROOT/$path" "$CANDIDATE_DIR/$path"
  else
    echo "Warning: Source path $path does not exist in studio"
  fi
done

sh "$CANDIDATE_DIR/scripts/open-core/rewrite-community-module-path.sh" "$CANDIDATE_DIR"

# Prune archived and launch doc trees from open-core candidate (TDE-1789629838711755000-bc3ed5d4)
rm -rf "$CANDIDATE_DIR/docs/launch"
if [ -d "$CANDIDATE_DIR/docs" ]; then
  find "$CANDIDATE_DIR/docs" -depth -type d \( -name "archive" -o -name "_archive" \) -exec rm -rf {} + 2>/dev/null || true
fi

# Prune excluded subpackages and builders per police-community-tree.sh
rm -rf "$CANDIDATE_DIR/pkg/mesh"
rm -rf "$CANDIDATE_DIR/pkg/agent"

if [ -d "$CANDIDATE_DIR/pkg/cli" ]; then
  find "$CANDIDATE_DIR/pkg/cli" \( \
    -name '*mesh*_command_builder.go' -o \
    -name '*keystore*_command_builder.go' -o \
    -name '*evolve*_command_builder.go' -o \
    -name '*agent_lockdown*_command_builder.go' -o \
    -name 'ambient_*_command_builder.go' -o \
    -name 'agent_*_command_builder.go' -o \
    -name '*paste*applescript*_command_builder.go' \
    \) -type f -delete 2>/dev/null || true
fi

# Also remove cmd/zqk-community/pkg_cmd/healthchk if it exists
rm -rf "$CANDIDATE_DIR/cmd/zqk-community/pkg_cmd/healthchk"

# Prune studio-specific system commands per police-community-tree.sh
rm -f "$CANDIDATE_DIR/cmd/zqk/system/agent_lockdown.go" \
      "$CANDIDATE_DIR/cmd/zqk/system/ambient_daemon.go" \
      "$CANDIDATE_DIR/cmd/zqk/system/sync_agents.go" \
      "$CANDIDATE_DIR/cmd/zqk/system/evolve.go" \
      "$CANDIDATE_DIR/cmd/zqk/system/materialize_agent_chat_channel.go" \
      "$CANDIDATE_DIR/cmd/zqk/system/materialize_agent_chat_channel_test.go"

# Stub studio command registration in candidate tree
if [ -f "$CANDIDATE_DIR/cmd/zqk/system/register_studio_commands.go" ]; then
  cat << 'EOF' > "$CANDIDATE_DIR/cmd/zqk/system/register_studio_commands.go"
package system

import "github.com/spf13/cobra"

// registerStudioCommands is a no-op in the open-core community edition.
func registerStudioCommands(_ *cobra.Command) {}
EOF
fi

# Clean overly broad patterns from community .gitignore
if [ -f "$CANDIDATE_DIR/.gitignore" ]; then
  grep -Ev '^\*-\*\.txt$|^\*test\*\.txt$|^\*_results\.txt$' "$CANDIDATE_DIR/.gitignore" > "$CANDIDATE_DIR/.gitignore.tmp" && mv "$CANDIDATE_DIR/.gitignore.tmp" "$CANDIDATE_DIR/.gitignore"
fi

sh "$CANDIDATE_DIR/scripts/open-core/prune-community-onboarding.sh" "$CANDIDATE_DIR"

if [ -x "$CANDIDATE_DIR/scripts/open-core/install-community-sku.sh" ]; then
  sh "$CANDIDATE_DIR/scripts/open-core/install-community-sku.sh" "$CANDIDATE_DIR"
fi

echo "Scrubbing internal studio kernel IDs (G15) from candidate tree..."
python3 - "$CANDIDATE_DIR" << 'EOF'
import sys, os, re

root = sys.argv[1]
pat = re.compile(r'(BLI|REQ|DEC|PRI|ATK|CRIT|TEST|GOAL|CVS)-[0-9]{13,}-[a-f0-9]{6,}')

for dirpath, dirnames, filenames in os.walk(root):
    if '.git' in dirpath:
        continue
    for fname in filenames:
        fpath = os.path.join(dirpath, fname)
        try:
            with open(fpath, 'r', encoding='utf-8', errors='ignore') as f:
                content = f.read()
            new_content, n = pat.subn(r'\1-REDACTED', content)
            if n > 0:
                with open(fpath, 'w', encoding='utf-8') as f:
                    f.write(new_content)
        except Exception:
            pass
EOF

echo "Initializing standalone git repository..."
git -C "$CANDIDATE_DIR" init -b main >/dev/null
git -C "$CANDIDATE_DIR" add .

echo "Running police checks..."
sh "$REPO_ROOT/scripts/open-core/police-community-tree.sh" "$CANDIDATE_DIR"

echo "Verifying payload gate..."
sh "$CANDIDATE_DIR/scripts/open-core/check-public-release-payload.sh" "$CANDIDATE_DIR"

echo "Candidate sync complete at $CANDIDATE_DIR"

