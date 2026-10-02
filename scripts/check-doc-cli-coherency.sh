#!/usr/bin/env bash
# scripts/check-doc-cli-coherency.sh
set -euo pipefail

MANIFEST=$(mktemp)
trap 'rm -f "$MANIFEST"' EXIT

echo "Extracting CLI manifest..."
./bin/zqk --help | awk '/^Available Commands:|^[[:alpha:]].*Commands:/{flag=1; next} /^[[:alpha:]].*Flags:/{flag=0} flag {print}' \
    | grep -E '^[[:space:]]{2,}[a-z][a-z0-9-]*[[:space:]]+' \
    | awk '{print $1}' > "$MANIFEST"

echo "Scanning documentation for zqk command references..."
PHANTOMS=0
MD_FILES=$(find . -name "*.md" -not -path "*/\.*" -not -path "*/node_modules/*" -not -path "./dist-docs/*" || true)

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
                    if ./bin/zqk ${current_cmd#zqk } --help 2>&1 | grep -q "unknown command"; then
                        echo "Phantom command found in $file: '$current_cmd' (extracted from '$doc_cmd')"
                        PHANTOMS=1
                        break
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
