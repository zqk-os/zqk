#!/bin/sh
# Fail-closed: community/public tree + package-community archive must not ship EE.
# Studio may still carry cmd/zqk-admin as proprietary carve-out — use mode studio-ee-allowed.
#
# Usage:
#   check-ee-packaging-boundary.sh [<root>] [community|studio-ee-allowed|packaging]
#   check-ee-packaging-boundary.sh --self-test
# Modes:
#   community         — EE module paths must be absent (public/PC tree)
#   studio-ee-allowed — studio must retain cmd/zqk-admin carve-out
#   packaging         — allow EE in tree; enforce package-community BINARIES + dist archive exclude
#
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — EE packaging boundary / stranger extract proof
set -eu

say() { printf '%s\n' "$*"; }
fail_msg() { say "FAIL: $*"; FAIL=1; }

run_self_test() {
	TMP=$(mktemp -d "${TMPDIR:-/tmp}/zqk-ee-boundary.XXXXXX")
	trap 'rm -rf "$TMP"' EXIT HUP INT TERM
	mkdir -p "$TMP/cmd" "$TMP/scripts/open-core" "$TMP/dist-community"
	# Minimal package-community that wrongly includes EE
	cat >"$TMP/scripts/package-community.sh" <<'EOF'
#!/bin/bash
BINARIES=(
  "zqk-community:./cmd/zqk"
  "zqk-admin:./cmd/zqk-admin"
)
EOF
	printf '%s\n' 'package main' >"$TMP/cmd/zqk-admin"
	# rewrite + payload stubs without refuse → community mode should fail on path presence
	printf '%s\n' '#!/bin/sh' >"$TMP/scripts/open-core/rewrite-community-module-path.sh"
	printf '%s\n' '#!/bin/sh' >"$TMP/scripts/open-core/check-public-release-payload.sh"
	# Fake archive containing EE binary name
	mkdir -p "$TMP/dist-community/stage"
	printf 'x' >"$TMP/dist-community/stage/zqk-admin"
	(cd "$TMP/dist-community" && tar -czf leak.tar.gz stage)

	if sh "$0" "$TMP" community >/dev/null 2>&1; then
		say "SELF-TEST FAIL: community mode accepted EE path + package-community EE binary"
		exit 1
	fi
	say "OK: self-test community mode fails closed on EE leak"
	say "EE PACKAGING BOUNDARY SELF-TEST: PASS"
	exit 0
}

if [ "${1:-}" = "--self-test" ]; then
	run_self_test
fi

ROOT=${1:-}
MODE=${2:-community}
if [ -z "$ROOT" ]; then
	ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)
FAIL=0

say "# EE packaging boundary"
say "root=$ROOT mode=$MODE"

for path in cmd/zqk-admin cmd/zqk-neuron; do
	if [ -e "$ROOT/$path" ]; then
		if [ "$MODE" = "studio-ee-allowed" ]; then
			say "OK: studio EE present: $path"
		elif [ "$MODE" = "packaging" ]; then
			say "OK: packaging mode tolerates EE path in tree: $path (BINARIES/archive still gated)"
		else
			fail_msg "community/public tree must not contain EE module path: $path"
		fi
	else
		if [ "$MODE" = "studio-ee-allowed" ] && [ "$path" = "cmd/zqk-admin" ]; then
			fail_msg "studio expected to retain proprietary $path (carve-out missing)"
		else
			say "OK: absent $path"
		fi
	fi
done

# package-community BINARIES must not reference EE modules (stranger extract contract)
PKG="$ROOT/scripts/package-community.sh"
if [ -f "$PKG" ]; then
	if grep -E 'zqk-admin|zqk-neuron|/cmd/zqk-admin|/cmd/zqk-neuron' "$PKG" >/dev/null 2>&1; then
		fail_msg "package-community.sh references EE module (zqk-admin/zqk-neuron)"
	else
		say "OK: package-community.sh has no EE binary/module refs"
	fi
	# Prefer explicit community-only binary paths when BINARIES= is present
	if grep -E 'BINARIES=\(' "$PKG" >/dev/null 2>&1; then
		if grep -E 'BINARIES=\(' -A20 "$PKG" | grep -E 'zqk-admin|zqk-neuron' >/dev/null 2>&1; then
			fail_msg "package-community BINARIES includes EE target"
		else
			say "OK: package-community BINARIES excludes EE targets"
		fi
	fi
else
	say "SKIP: package-community.sh absent"
fi

# Community archive / export rewrite refuse contract
if [ -f "$ROOT/scripts/open-core/rewrite-community-module-path.sh" ]; then
	if grep -F 'cmd/zqk-admin' "$ROOT/scripts/open-core/rewrite-community-module-path.sh" >/dev/null 2>&1; then
		say "OK: rewrite-community-module-path refuses zqk-admin"
	else
		fail_msg "rewrite-community-module-path.sh missing zqk-admin refuse guard"
	fi
fi

# Payload gate must list EE excludes when present
if [ -f "$ROOT/scripts/open-core/check-public-release-payload.sh" ]; then
	if grep -F 'cmd/zqk-admin' "$ROOT/scripts/open-core/check-public-release-payload.sh" >/dev/null 2>&1; then
		say "OK: check-public-release-payload forbids cmd/zqk-admin"
	else
		fail_msg "check-public-release-payload.sh missing cmd/zqk-admin forbid"
	fi
fi

# Stranger extract / dist archive proof: no EE names inside packaged tarballs
DIST="${ZQK_DIST_DIR:-$ROOT/dist-community}"
if [ -d "$DIST" ]; then
	FOUND=""
	for archive in "$DIST"/*.tar.gz "$DIST"/*.tgz; do
		[ -f "$archive" ] || continue
		if tar -tzf "$archive" 2>/dev/null | grep -E '(^|/)(zqk-admin|zqk-neuron)(/|$)' >/dev/null 2>&1; then
			FOUND="${FOUND}${archive} "
		fi
	done
	if [ -n "$FOUND" ]; then
		fail_msg "community dist archive contains EE binary path(s): $FOUND"
	else
		say "OK: dist-community archives (if any) exclude EE binary names"
	fi
else
	say "SKIP: dist-community absent (tree-level + package-community contract still enforced)"
fi

if [ "$FAIL" -ne 0 ]; then
	say "EE PACKAGING BOUNDARY: FAIL"
	exit 1
fi
say "EE PACKAGING BOUNDARY: PASS"
exit 0
