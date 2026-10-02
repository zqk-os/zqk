#!/usr/bin/env bash
# scripts/check-doc-links.sh
set -euo pipefail

BROKEN_LINKS=0
MD_FILES=$(find . -name "*.md" -not -path "*/\.*" -not -path "*/node_modules/*" -not -path "./dist-docs/*" || true)

echo "Scanning for broken internal links in documentation..."

for file in $MD_FILES; do
    dir=$(dirname "$file")
    
    LINKS=$(grep -oE '\]\([^)]+\)' "$file" | sed -e 's/^\](//' -e 's/)$//' || true)
    
    while IFS= read -r link; do
        [ -z "$link" ] && continue
        
        if [[ "$link" == http* ]] || [[ "$link" == \#* ]] || [[ "$link" == mailto:* ]]; then
            continue
        fi
        
        file_path="${link%%#*}"
        [ -z "$file_path" ] && continue
        
        if [[ "$file_path" == /* ]]; then
            target=".$file_path"
        else
            target="$dir/$file_path"
        fi
        
        if [ ! -e "$target" ] && [ ! -d "$target" ]; then
            echo "Broken link found in $file: '$link' (resolved to $target)"
            BROKEN_LINKS=1
        fi
    done <<< "$LINKS"
done

if [ $BROKEN_LINKS -ne 0 ]; then
    echo "Validation failed: Broken internal links found."
    exit 1
else
    echo "Validation passed: All internal links resolve correctly."
    exit 0
fi
