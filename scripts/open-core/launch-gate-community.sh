#!/usr/bin/env bash
# launch-gate-community.sh — stranger launch honesty meta-gate for public-candidate.
#
# Composes the fail-closed surface a cold clone must pass before launch claims:
#   1) G15 studio-ID scrub (product Go)
#   2) public-release payload (forbidden trees / ontology)
#   3) police-community-tree (seed + paging + bounded whats-next + EE boundary)
#   4) ttfa-unauth-smoke (optional; default on) — install→init→whats-next ≤120s, no tokens
#
# Usage:
#   sh scripts/open-core/launch-gate-community.sh [candidate-root]
#   ZQK_BIN=/path/to/zqk SKIP_TTFA=1 sh scripts/open-core/launch-gate-community.sh .
#
# TRACK: OPEN_CORE_PUBLIC_RELEASE_HYGIENE — stranger launch-gate residual
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="${1:-}"
if [ -z "$ROOT" ]; then
	ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
else
	ROOT="$(CDPATH= cd -- "$ROOT" && pwd)"
fi

say() { printf '%s\n' "$*"; }
fail() { say "FAIL: $*" >&2; exit 1; }

[ -d "$ROOT" ] || fail "candidate root missing: $ROOT"
[ -x "$ROOT/scripts/open-core/check-g15-studio-id-scrub.sh" ] || fail "missing G15 scrub under $ROOT"
[ -x "$ROOT/scripts/open-core/check-public-release-payload.sh" ] || [ -f "$ROOT/scripts/open-core/check-public-release-payload.sh" ] || fail "missing payload gate under $ROOT"
[ -f "$ROOT/scripts/open-core/police-community-tree.sh" ] || fail "missing police under $ROOT"

say "# community launch-gate"
say "root=$ROOT"

sh "$ROOT/scripts/open-core/check-g15-studio-id-scrub.sh" "$ROOT" || fail "G15 scrub"
sh "$ROOT/scripts/open-core/check-public-release-payload.sh" "$ROOT" || fail "public-release payload"
ZQK_BIN="${ZQK_BIN:-$ROOT/bin/zqk}" sh "$ROOT/scripts/open-core/police-community-tree.sh" "$ROOT" || fail "police-community-tree"

if [ "${SKIP_TTFA:-0}" != "1" ]; then
	[ -f "$ROOT/scripts/open-core/ttfa-unauth-smoke.sh" ] || fail "missing ttfa-unauth-smoke under $ROOT"
	# Prefer tree-local community bin when present so TTFA does not pick a studio stub.
	if [ -z "${ZQK_BIN:-}" ] && [ -x "$ROOT/bin/zqk" ]; then
		export ZQK_BIN="$ROOT/bin/zqk"
	fi
	unset GITHUB_TOKEN GH_TOKEN || true
	KEEP_TTFA_WORKDIR="${KEEP_TTFA_WORKDIR:-0}" \
		sh "$ROOT/scripts/open-core/ttfa-unauth-smoke.sh" || fail "ttfa-unauth-smoke"
else
	say "SKIP_TTFA=1 — skipped unauth TTFA smoke"
fi

say "LAUNCH_GATE_COMMUNITY: PASS"
exit 0
