#!/usr/bin/env bash
# scripts/check-doc-links.sh
set -euo pipefail

BROKEN_LINKS=0
# Exclude build artifacts, vendor, and hidden dirs
MD_FILES=$(find . -name "*.md" -not -path "*/\.*" -not -path "*/node_modules/*" -not -path "./dist-docs/*" -not -path "./dist-community/*" -not -path "./vendor/*" || true)

echo "Scanning for broken internal links in documentation..."

for file in $MD_FILES; do
    dir=$(dirname "$file")
    
    LINKS=$(grep -oE '\]\([^)]+\)' "$file" | sed -e 's/^\](//' -e 's/)$//' || true)
    
    while IFS= read -r link; do
        [ -z "$link" ] && continue
        
        # Skip external links, anchors, mailto
        if [[ "$link" == http* ]] || [[ "$link" == \#* ]] || [[ "$link" == mailto:* ]]; then
            continue
        fi
        
        # Skip non-path strings (code snippets like [T](func(T)), strings with spaces or quotes)
        if [[ "$link" =~ [[:space:]] ]] || [[ "$link" == *'"'* ]] || [[ "$link" == *"("* ]] || [[ "$link" == *")"* ]]; then
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
            # SKU overlay templates have links written relative to their installed destinations:
            # - README.md -> repo root
            # - DOCS_INDEX.md, GETTING_STARTED.md -> docs/
            # - ARCHITECTURE_*.md -> docs/architecture/
            # - COMMUNITY_FIRST_RUN.md -> docs/onboarding/
            if [[ "$dir" == *"sku-overlay"* ]]; then
                if [ -e "./$file_path" ] || [ -d "./$file_path" ] || \
                   [ -e "./docs/$file_path" ] || [ -d "./docs/$file_path" ] || \
                   [ -e "./docs/architecture/$file_path" ] || [ -d "./docs/architecture/$file_path" ] || \
                   [ -e "./docs/onboarding/$file_path" ] || [ -d "./docs/onboarding/$file_path" ]; then
                    continue
                fi
            fi
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
