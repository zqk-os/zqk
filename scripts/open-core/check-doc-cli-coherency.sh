#!/usr/bin/env bash
# scripts/check-doc-cli-coherency.sh
set -euo pipefail

MANIFEST=$(mktemp)
trap 'rm -f "$MANIFEST"' EXIT

echo "Extracting CLI manifest..."
# Extract ALL commands from help output — handles varied section headers
# (Getting Started, Everyday Commands, Integrations, Advanced, etc.)
./bin/zqk --help \
    | grep -E '^  [a-z][a-z0-9-]*[[:space:]]+' \
    | awk '{print $1}' \
    | sort -u > "$MANIFEST"

# Also add known aliases and shorthand commands
./bin/zqk --help | grep -oE 'zqk [a-z][a-z0-9-]*' | awk '{print $2}' | sort -u >> "$MANIFEST"
# Well-known root command aliases
printf "%s\n" "auto-exec" "auto-do" "glossary" "zgrep" "plan" >> "$MANIFEST"
sort -u -o "$MANIFEST" "$MANIFEST"

CMD_COUNT=$(wc -l < "$MANIFEST" | tr -d ' ')
echo "  Found $CMD_COUNT commands/aliases in manifest"

echo "Scanning documentation for zqk command references..."
PHANTOMS=0
# Exclude build artifacts (dist-community, dist-docs), hidden dirs, and vendor
MD_FILES=$(find . -name "*.md" -not -path "*/\.*" -not -path "*/node_modules/*" -not -path "./dist-docs/*" -not -path "./dist-community/*" -not -path "./vendor/*" || true)

VALID_CACHE=$(mktemp)
INVALID_CACHE=$(mktemp)
trap 'rm -f "$MANIFEST" "$VALID_CACHE" "$INVALID_CACHE"' EXIT

for file in $MD_FILES; do
    COMMANDS_IN_DOC=$(grep -oE '\`zqk [a-z0-9-][a-z0-9 -]*\`' "$file" | tr -d '`' || true)
    
    while IFS= read -r doc_cmd; do
        [ -z "$doc_cmd" ] && continue
        
        words=($doc_cmd)
        root_cmd="${words[1]:-}"
        
        if [ -n "$root_cmd" ] && [[ "$root_cmd" =~ ^[a-z0-9-]+$ ]]; then
            if ! grep -Fxq "$root_cmd" "$MANIFEST"; then
                echo "Phantom command found in $file: 'zqk $root_cmd' (extracted from '$doc_cmd')"
                PHANTOMS=1
            else
                current_cmd="zqk $root_cmd"
                for ((i=2; i<${#words[@]}; i++)); do
                    word="${words[$i]}"
                    if [[ ! "$word" =~ ^[a-z0-9-]+$ ]]; then break; fi
                    
                    current_cmd="$current_cmd $word"

                    if grep -Fxq "$current_cmd" "$VALID_CACHE" 2>/dev/null; then
                        continue
                    fi
                    if grep -Fxq "$current_cmd" "$INVALID_CACHE" 2>/dev/null; then
                        echo "Phantom command found in $file: '$current_cmd' (extracted from '$doc_cmd')"
                        PHANTOMS=1
                        break
                    fi

                    if ./bin/zqk ${current_cmd#zqk } --help 2>&1 | grep -q "unknown command"; then
                        echo "$current_cmd" >> "$INVALID_CACHE"
                        echo "Phantom command found in $file: '$current_cmd' (extracted from '$doc_cmd')"
                        PHANTOMS=1
                        break
                    else
                        echo "$current_cmd" >> "$VALID_CACHE"
                    fi
                done
            fi
        fi
    done <<< "$COMMANDS_IN_DOC"
done

if [ $PHANTOMS -ne 0 ]; then
    echo "Validation failed: Phantom CLI features found in docs."
    exit 1
else
    echo "Validation passed: Docs and CLI are coherent."
    exit 0
fi
