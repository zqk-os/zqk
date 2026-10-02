#!/bin/bash
set -e

MANIFEST_DIR="scripts/open-core/sku-overlay/config"
MANIFEST_FILE="${MANIFEST_DIR}/community-capabilities.yaml"

mkdir -p "$MANIFEST_DIR"

cat <<INNEREOF > "$MANIFEST_FILE"
# Auto-generated from binary introspection. Do not edit manually.
# Regenerate: ./scripts/open-core/generate-sku-manifest.sh
sku: community
generated_at: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
top_level_commands:
INNEREOF

./bin/zqk --help | awk '
/^Getting Started:/, /^Additional Commands:/ {
    if ($0 ~ /^  [a-z-]/) {
        cmd = $1
        sub(/^  [^ ]+ +/, "")
        print cmd "|" $0
    }
}' | while IFS='|' read -r cmd desc; do
    echo "  - name: $cmd" >> "$MANIFEST_FILE"
    echo "    description: \"$desc\"" >> "$MANIFEST_FILE"
    
    if [[ "$cmd" == "auth" || "$cmd" == "system" || "$cmd" == "agent" || "$cmd" == "mcp" ]]; then
        echo "    subcommands:" >> "$MANIFEST_FILE"
        ./bin/zqk "$cmd" --help | awk '
        /^Available Commands:/, /^(Global )?Flags:/ {
            if ($0 ~ /^  [a-z-]/) {
                print "      - " $1
            }
        }' >> "$MANIFEST_FILE"
    fi
done

echo "Generated $MANIFEST_FILE"
