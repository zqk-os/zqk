#!/bin/sh
set -eu

ROOT=${1:-}
if [ -z "$ROOT" ]; then
	ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)
TMP_DEST=$(mktemp -d "${TMPDIR:-/tmp}/zqk-community-sku.XXXXXX")
EXPECTED_LICENSE=$(mktemp "${TMPDIR:-/tmp}/zqk-community-license.XXXXXX")
trap 'rm -rf "$TMP_DEST"; rm -f "$EXPECTED_LICENSE"' EXIT HUP INT TERM

sh -n "$ROOT/scripts/open-core/apply-community-source.sh"
sh -n "$ROOT/scripts/open-core/sync-public-candidate.sh"
sh -n "$ROOT/scripts/open-core/install-community-sku.sh"

awk '
	/^--------------------------------------------------------------------------------$/ { found_separator=1; exit }
	{ lines[NR]=$0 }
	END {
		last=NR-found_separator
		while (last > 0 && lines[last] == "") last--
		for (i=1; i<=last; i++) print lines[i]
	}
' "$ROOT/LICENSE" >"$EXPECTED_LICENSE"
sh "$ROOT/scripts/open-core/install-community-sku.sh" "$TMP_DEST" >/dev/null

cmp "$EXPECTED_LICENSE" "$TMP_DEST/LICENSE"
cmp "$ROOT/scripts/open-core/sku-overlay/NOTICE" "$TMP_DEST/NOTICE"
cmp "$ROOT/scripts/open-core/sku-overlay/SECURITY.md" "$TMP_DEST/SECURITY.md"
cmp "$ROOT/scripts/open-core/sku-overlay/CI.yml" "$TMP_DEST/.github/workflows/ci.yml"
cmp "$ROOT/scripts/open-core/sku-overlay/config/zqk.yaml" "$TMP_DEST/config/zqk.yaml"
cmp "$ROOT/scripts/open-core/sku-overlay/README.md" "$TMP_DEST/README.md"
cmp "$ROOT/scripts/open-core/sku-overlay/DOCS_INDEX.md" "$TMP_DEST/docs/INDEX.md"
cmp "$ROOT/scripts/open-core/sku-overlay/COMMUNITY_FIRST_RUN.md" "$TMP_DEST/docs/onboarding/COMMUNITY_FIRST_RUN.md"

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
