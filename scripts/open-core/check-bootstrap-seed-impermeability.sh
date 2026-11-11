#!/bin/sh
# Fail-closed: community bootstrap archive / init seed manifest must not embed
# CAP/MMORCH/orchestrate/TPM operator ontology (seed impermeability).
#
# Usage:
#   check-bootstrap-seed-impermeability.sh [ROOT]
#   check-bootstrap-seed-impermeability.sh --self-test
#
# Looks for pkg/bootstrap/archive/manifest.txt (preferred) or lists
# pkg/bootstrap/archive/bootstrap.tar.gz. Fragments MUST stay aligned with
# pkg/bootstrap/community_denylist.go (CommunityBootstrapDenylist).
set -eu

ROOT=""
SELF_TEST=0
while [ $# -gt 0 ]; do
	case "$1" in
		--self-test)
			SELF_TEST=1
			shift
			;;
		-*)
			echo "usage: $0 [--self-test] [ROOT]" >&2
			exit 2
			;;
		*)
			ROOT=$1
			shift
			;;
	esac
done

if [ -z "$ROOT" ]; then
	ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)
fi
ROOT=$(CDPATH= cd -- "$ROOT" && pwd)

# Keep in sync with pkg/bootstrap/community_denylist.go communityBootstrapDenylist.
DENY_FRAGMENTS='
print_cursor_paste_applescript
print_ide_paste_applescript
paste_applescript
record_cvs_orchestrate_run
paste_cursor
zqk_cursor
cap_night_duty
cap_orchestrator
cap_background
mmorch
orchestrate_command
orchestrate_batch
tpm_orchestration
/agent/orchestrate
'

scan_paths() {
	_paths=$1
	_label=$2
	_hits=""
	for frag in $DENY_FRAGMENTS; do
		case "$_paths" in
			*"$frag"*)
				_match=$(printf '%s\n' "$_paths" | grep -F "$frag" || true)
				if [ -n "$_match" ]; then
					_hits="${_hits}
DENY ($frag):
$_match"
				fi
				;;
		esac
	done
	if [ -n "$_hits" ]; then
		printf '%s\n' "BLOCK: community bootstrap seed impermeability failed ($_label)"
		printf '%s\n' "$_hits"
		return 1
	fi
	return 0
}

if [ "$SELF_TEST" -eq 1 ]; then
	BAD=$(mktemp "${TMPDIR:-/tmp}/zqk-seed-deny-bad.XXXXXX")
	GOOD=$(mktemp "${TMPDIR:-/tmp}/zqk-seed-deny-good.XXXXXX")
	trap 'rm -f "$BAD" "$GOOD"' EXIT HUP INT TERM
	printf '%s\n' \
		'./cli_specs/agent/orchestrate_command.yaml' \
		'./scripts/scheduler_jobs/cap_orchestrator.yaml' \
		'./scripts/default_agent_skills/tpm_orchestration.yaml' >"$BAD"
	printf '%s\n' \
		'./objects/mission.yaml' \
		'./cli_specs/scheduler/activity_command.yaml' \
		'./scripts/scheduler_jobs/cleanup.yaml' >"$GOOD"
	if scan_paths "$(cat "$BAD")" "self-test-bad"; then
		echo "self-test: expected FAIL on denylist fixture" >&2
		exit 1
	fi
	if ! scan_paths "$(cat "$GOOD")" "self-test-good"; then
		echo "self-test: expected PASS on clean fixture" >&2
		exit 1
	fi
	echo "SEED IMPERMEABILITY SELF-TEST: PASS"
	exit 0
fi

MANIFEST=""
ARCHIVE=""
if [ -f "$ROOT/pkg/bootstrap/archive/manifest.txt" ]; then
	MANIFEST="$ROOT/pkg/bootstrap/archive/manifest.txt"
fi
if [ -f "$ROOT/pkg/bootstrap/archive/bootstrap.tar.gz" ]; then
	ARCHIVE="$ROOT/pkg/bootstrap/archive/bootstrap.tar.gz"
fi
# Studio community embed path (optional; only when present alongside community layout).
if [ -z "$MANIFEST" ] && [ -f "$ROOT/internal/bootstrap/archive/manifest.txt" ] && [ -f "$ROOT/scripts/open-core/package-community.sh" ]; then
	MANIFEST="$ROOT/internal/bootstrap/archive/manifest.txt"
fi
if [ -z "$ARCHIVE" ] && [ -f "$ROOT/internal/bootstrap/archive/bootstrap.tar.gz" ] && [ -f "$ROOT/scripts/open-core/package-community.sh" ]; then
	ARCHIVE="$ROOT/internal/bootstrap/archive/bootstrap.tar.gz"
fi

if [ -z "$MANIFEST" ] && [ -z "$ARCHIVE" ]; then
	echo "BLOCK: no bootstrap manifest.txt or bootstrap.tar.gz under pkg/bootstrap/archive (or community embed)" >&2
	exit 1
fi

PATHS=""
if [ -n "$MANIFEST" ]; then
	PATHS=$(cat "$MANIFEST")
	LABEL="manifest.txt"
elif [ -n "$ARCHIVE" ]; then
	PATHS=$(tar -tzf "$ARCHIVE")
	LABEL="bootstrap.tar.gz"
fi

echo "# bootstrap seed impermeability"
echo "root=$ROOT"
echo "source=$LABEL"

if ! scan_paths "$PATHS" "$LABEL"; then
	echo "SEED IMPERMEABILITY: FAIL"
	exit 1
fi

# Cross-check archive listing when both exist (drift between manifest and tar).
if [ -n "$MANIFEST" ] && [ -n "$ARCHIVE" ]; then
	TAR_PATHS=$(tar -tzf "$ARCHIVE")
	if ! scan_paths "$TAR_PATHS" "bootstrap.tar.gz"; then
		echo "SEED IMPERMEABILITY: FAIL (archive listing)"
		exit 1
	fi
fi

echo "SEED IMPERMEABILITY: PASS"
exit 0
