#!/bin/sh
set -eu

ROOT=${1:-}
if [ -z "$ROOT" ]; then
	ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)
TMP_DEST=$(mktemp -d "${TMPDIR:-/tmp}/zqk-community-sku.XXXXXX")
trap 'rm -rf "$TMP_DEST"' EXIT HUP INT TERM

sh -n "$ROOT/scripts/open-core/apply-community-source.sh"
sh -n "$ROOT/scripts/open-core/sync-public-candidate.sh"
sh -n "$ROOT/scripts/open-core/install-community-sku.sh"

sh "$ROOT/scripts/open-core/install-community-sku.sh" "$TMP_DEST" >/dev/null

cmp "$ROOT/LICENSE" "$TMP_DEST/LICENSE"
cmp "$ROOT/NOTICE" "$ROOT/scripts/open-core/sku-overlay/NOTICE"
cmp "$ROOT/scripts/open-core/sku-overlay/NOTICE" "$TMP_DEST/NOTICE"
cmp "$ROOT/SECURITY.md" "$ROOT/scripts/open-core/sku-overlay/SECURITY.md"
cmp "$ROOT/scripts/open-core/sku-overlay/SECURITY.md" "$TMP_DEST/SECURITY.md"
cmp "$ROOT/.github/workflows/ci.yml" "$ROOT/scripts/open-core/sku-overlay/CI.yml"
cmp "$ROOT/scripts/open-core/sku-overlay/CI.yml" "$TMP_DEST/.github/workflows/ci.yml"
cmp "$ROOT/README.md" "$ROOT/scripts/open-core/sku-overlay/README.md"
cmp "$ROOT/scripts/open-core/sku-overlay/README.md" "$TMP_DEST/README.md"

for path in \
	scripts/open-core/apply-community-source.sh \
	scripts/open-core/sync-public-candidate.sh \
	scripts/open-core/community-bounded-includes \
	scripts/open-core/community-bounded-includes.txt; do
	if grep -F '.goreleaser.yaml' "$ROOT/$path" >/dev/null; then
		printf 'stale release config remains in %s\n' "$path" >&2
		exit 1
	fi
done

grep -F 'install-community-sku.sh' "$ROOT/scripts/open-core/sync-public-candidate.sh" >/dev/null
grep -F 'scripts/open-core/check-public-release-payload.sh' "$ROOT/scripts/open-core/sync-public-candidate.sh" >/dev/null

printf '%s\n' "COMMUNITY OVERLAY RELEASE FILES: PASS"
