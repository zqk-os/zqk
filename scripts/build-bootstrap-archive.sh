#!/usr/bin/env sh
# Builds the bootstrap archive from .zqk/specs, .zqk/cli/specs, and scripts/scheduler_jobs (maintenance templates) for embedding.
# Run via: make bootstrap-archive
# REQ-9009: Build process creates bootstrap archive.
# REQ-9011: Manifest provides traceability for bundled files.
set -e
export LC_ALL=C
REPO_ROOT="${1:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}"
REPO_ROOT="$(cd "$REPO_ROOT" && pwd)"
if [ -d "${REPO_ROOT}/.zqk/specs" ]; then
	INTERNAL_DIR="${REPO_ROOT}/.zqk/specs"
elif [ -d "${REPO_ROOT}/.zqk/specs" ]; then
	INTERNAL_DIR="${REPO_ROOT}/.zqk/specs"
else
	echo "Error: specs source directory not found (.zqk/specs or .zqk/specs)" >&2
	exit 1
fi
CLI_SPECS_DIR="${REPO_ROOT}/.zqk/cli/specs"
SCRIPTS_SCHEDULER_JOBS_DIR="${REPO_ROOT}/scripts/scheduler_jobs"
ARCHIVE_DIR="${REPO_ROOT}/internal/bootstrap/archive"
ARCHIVE="${ARCHIVE_DIR}/bootstrap.tar.gz"
MANIFEST="${ARCHIVE_DIR}/manifest.txt"

mkdir -p "$ARCHIVE_DIR"
# Staging: _internal at root, cli_specs under cli_specs/, scripts/scheduler_jobs under scripts/ for extract mapping
STAGING="${TMPDIR:-/tmp}/zqk-bootstrap-staging.$$"
mkdir -p "$STAGING"
trap 'rm -rf "$STAGING"' EXIT

# macOS: avoid copying AppleDouble sidecars; COPYFILE_DISABLE also suppresses creating new ones during cp.
export COPYFILE_DISABLE=1

cp -r "$INTERNAL_DIR"/* "$STAGING/" 2>/dev/null || true
if [ -d "$CLI_SPECS_DIR" ]; then
	mkdir -p "$STAGING/cli_specs"
	cp -r "$CLI_SPECS_DIR"/* "$STAGING/cli_specs/" 2>/dev/null || true
fi
# Maintenance job templates so init --with-maintenance-jobs works in greenfield (no repo scripts to copy from)
if [ -d "$SCRIPTS_SCHEDULER_JOBS_DIR" ]; then
	mkdir -p "$STAGING/scripts/scheduler_jobs"
	for f in retention_tolerance_catchall.yaml audit_event_aggregation_default.yaml onboarding_roadmap_seed.yaml cleanup_on_demand.yaml; do
		[ -f "$SCRIPTS_SCHEDULER_JOBS_DIR/$f" ] && cp "$SCRIPTS_SCHEDULER_JOBS_DIR/$f" "$STAGING/scripts/scheduler_jobs/"
	done
fi
# Default policy pack templates (seeded by system init)
DEFAULT_POLICIES_DIR="${REPO_ROOT}/scripts/default_policies"
if [ -d "$DEFAULT_POLICIES_DIR" ]; then
	mkdir -p "$STAGING/scripts/default_policies"
	cp -r "$DEFAULT_POLICIES_DIR"/* "$STAGING/scripts/default_policies/" 2>/dev/null || true
fi
# Community default personas + agent_skills (seeded by system init for feed/chat)
for pack in default_personas default_agent_skills; do
	SRC="${REPO_ROOT}/scripts/${pack}"
	if [ -d "$SRC" ]; then
		mkdir -p "$STAGING/scripts/${pack}"
		cp -r "$SRC"/* "$STAGING/scripts/${pack}/" 2>/dev/null || true
	fi
done
# Cleanup config: extract to .zqk/cleanup/config.yaml so SCH-cleanup job has single-track input after init
if [ -f "${INTERNAL_DIR}/configs/cleanup_config.yaml" ]; then
	mkdir -p "$STAGING/cleanup_config"
	cp "${INTERNAL_DIR}/configs/cleanup_config.yaml" "$STAGING/cleanup_config/config.yaml"
fi
# Onboarding roadmap templates (priority plan, workstream, backlog items); seed job creates them when run
ONBOARDING_DIR="${REPO_ROOT}/scripts/onboarding_roadmap"
if [ -d "$ONBOARDING_DIR" ]; then
	mkdir -p "$STAGING/scripts/onboarding_roadmap"
	for f in priority_plan_onboarding.yaml workstream_onboarding.yaml backlog_item_01_read_philosophy.yaml backlog_item_02_start_here.yaml backlog_item_03_system_health.yaml milestone_onboarding.yaml goal_onboarding.yaml README.md; do
		[ -f "$ONBOARDING_DIR/$f" ] && cp "$ONBOARDING_DIR/$f" "$STAGING/scripts/onboarding_roadmap/"
	done
fi

# Drop any AppleDouble sidecars that were already in the source tree (COPYFILE_DISABLE does not remove existing ._ files).
find "$STAGING" -name '._*' -delete 2>/dev/null || true

# Open-core / community hygiene: never embed Studio live process instances.
# Specs/lifecycles/configs stay; instance-shaped trees and demo BLI YAML go.
rm -rf \
	"$STAGING/backlog_item" \
	"$STAGING/backlog" \
	"$STAGING/accounts" \
	"$STAGING/convergence_sessions" \
	"$STAGING/organizations" \
	"$STAGING/keystore" \
	"$STAGING/mcp_sessions" \
	"$STAGING/scheduler_jobs" \
	"$STAGING/audit" \
	"$STAGING/change_journal" \
	"$STAGING/metrics" \
	2>/dev/null || true
find "$STAGING" \( \
	-name 'BLI-*.yaml' -o \
	-name 'CVS-*.yaml' -o \
	-name 'ORG-*.yaml' -o \
	-name 'ACC-*.yaml' \
	\) -type f -delete 2>/dev/null || true

SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-$(git -C "$REPO_ROOT" log -1 --pretty=%ct 2>/dev/null || date +%s)}"
TOUCH_TS="$(date -u -r "$SOURCE_DATE_EPOCH" '+%Y%m%d%H%M.%S' 2>/dev/null || date -u '+%Y%m%d%H%M.%S')"
find "$STAGING" -exec touch -h -t "$TOUCH_TS" {} + 2>/dev/null || true

export GZIP="-n"
(cd "$STAGING" && find . | sort | tar -cf - -T - | gzip -n > "$ARCHIVE")
# Traceability: list all paths in the archive (REQ-9011)
tar tzf "$ARCHIVE" | sort > "$MANIFEST"
echo "Created $ARCHIVE ($(wc -l < "$MANIFEST") entries) and $MANIFEST"
