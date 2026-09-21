package paths

import (
	"io/fs"

	"github.com/zqk-os/zqk/pkg/brand"
)

// Project data directory constants.
// These are the default layout; the canonical source for "where are docs/zqk/cache" is brand settings
// (zqk-settings.yaml paths.aliases) and the path alias cache built from it. Use paths.ResolvePathFromCacheOrConstant
// or paths.ResolvePath(projectRoot, "prefix:<alias>") when building paths so moving folders only requires editing the settings file.
//
// Usage when cache may be built (prefer so settings can override):
//   paths.ResolvePathFromCacheOrConstant(projectRoot, "cache", filepath.Join(paths.ProjectDataDir, paths.CacheDir))
// Fallback when no project root or cache (e.g. init): filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir, ...)

var (
	// ProjectDataDir is the base directory name for project data
	// This is where all project-specific data, configuration, and state is stored
	ProjectDataDir = "." + brand.NamespacePrefix()

	// Metrics directory constants (under project data, not source tree)
	MetricsProfilesDir = ProjectDataDir + "/metrics/profiles"

	// Seed questions directory (under project data); see initialization-seed-questions-v1.0.md
	SeedQuestionsDir = ProjectDataDir + "/seed/questions"

	// CLICommandSpecsDir is the canonical file-authored command DNA directory.
	CLICommandSpecsDir = ProjectDataDir + "/cli/specs"

	// ProjectStateDir is the root directory name for compressed snapshot state (.zqk-state).
	ProjectStateDir = "." + brand.NamespacePrefix() + "-state"

	// AgentPacksDir holds regenerable per-vendor boot packs.
	AgentPacksDir = ProjectDataDir + "/agent_packs"
)

const (
	ConfigYAMLFileName = "config.yaml"
	configYAMLFileName = ConfigYAMLFileName

	// DefaultProjectStateDir is the literal directory name for project state snapshots (".zqk-state").
	DefaultProjectStateDir = ".zqk-state"

	// GitWorktreeMetadataEntry is the file or directory name Git places at the root of a work tree.
	// Tests and teardown use it to avoid stripping or scrubbing a real checkout.
	GitWorktreeMetadataEntry = ".git"

	// StudioNestedWorktreesDir is a repo-root folder some seats use for linked
	// worktrees. That violates POL-AGENT-WORKTREE-ISOLATION-001 (default is
	// $TMPDIR/zqk-worktrees/…). Test discovery must skip it so scan-tests
	// --package does not double-count the same tests from the nested tree.
	StudioNestedWorktreesDir = ".worktrees"

	// Common subdirectories under ProjectDataDir
	CacheDir = "cache"
	LogsDir  = "logs"
	// ConfigDir is the directory name "config".
	// Joined with projectRoot it is the committed product config tree (SSOT).
	// Joined with ProjectDataDir it is leftover kernel lite-files (.zqk/config).
	// Do not add new YAML there.
	// TRACK: docs/onboarding/COMMUNITY_FIRST_RUN.md — move agent/idle/chat/git-identity
	// lite-files to StateDir when: no reader still joins ProjectDataDir+ConfigDir for YAML.
	ConfigDir = "config"

	// Canonical configuration files under ConfigDir ("config") at project root.
	ZqkConfigFileName      = "zqk.yaml"
	ZqkLocalConfigFileName = "zqk-local.yaml"
	ZqkTestConfigFileName  = "zqk-test.yaml"
	ZqkEnvConfigFilePrefix = "zqk-"

	MCPDir = "mcp"
	// WorkshopBinDir holds the workshop stable CLI binary under ProjectDataDir (e.g. .zqk/bin/<exe>-stable).
	WorkshopBinDir = "bin"
	// RepoBinDir is the repository-root bin/ directory (compiled CLI for local ops).
	RepoBinDir = "bin"
	// MeshStateSubdir holds local mesh runtime state under ProjectDataDir/state/ (peer seats, peer-ack awaits).
	MeshStateSubdir = "mesh"
	// SwarmInitSubdir holds swarm-init run artifacts under MeshStateSubdir (date-bucketed).
	SwarmInitSubdir = "swarm_init"
	MetricsDir      = "metrics"
	StateDir        = "state"
	// SessionStateFile is the persisted CLI session id under StateDir.
	SessionStateFile = "session"
	// CredentialsFile is the home-dir session token filename under ProjectDataDir.
	CredentialsFile       = "credentials"
	StreamCurrentSubdir   = "stream_current" // Runtime deltas for stream-backed kinds (no CAS hash); see HIGH_VOLUME_STORAGE_DEPRECATION.md
	CallbackDir           = "callback-logs"
	MigrationSnapshotsDir = "migration-snapshots"
	WalDir                = "wal"
	ProcessSubdir         = "process"
	SpecsSubdir           = "specs"
	SkillsSubdir          = "skills"
	IdesSubdir            = "ides"
	InboxSubdir           = "inbox"
	SchedulerSubdir       = "scheduler"
	StreamsSubdir         = "streams"
	WorktreesSubdir       = "worktrees"
	RunSubdir             = "run"
	DraftsSubdir          = "drafts"
	CleanupSubdir         = "cleanup"
	AgentPacksSubdir      = "agent_packs"

	// Logs subdirs: reports (object-count-report, etc.) live under logs/reports/
	LogsReportsSubdir = "reports"
	// LogsDriftSubdir holds convergence rollup/drift tooling output (default CVS_ORCH_ROLLUP_OUT directory segment).
	// Full path: ProjectDataDir/LogsDir/LogsDriftSubdir — teams may override output via CVS_ORCH_ROLLUP_OUT on orchestrate jobs.
	LogsDriftSubdir = "drift"
	// LLMLogsSubdir holds swarm prompt/response traces under ProjectDataDir/LogsDir.
	LLMLogsSubdir = "llm"
	// Reports index and dashboard snapshot (object-count-report): durable index and latest snapshot for aggregation and dashboards.
	ReportsIndexFile              = "reports_index.json"
	LatestObjectCountSnapshotFile = "latest_object_count_snapshot.json"
	// Metrics subdirs: object_volume time series (from object-count-report) live under metrics/object_volume/
	MetricsObjectVolumeSubdir = "object_volume"
	// Metrics subdirs: stream_volume time series (from high-volume stream kinds like audit_event) live under metrics/stream_volume/
	MetricsStreamVolumeSubdir = "stream_volume"
	// MetricsFilesystemSnapshotSubdir: full-tree file/byte counts by bucket (from object-count-report --include-filesystem-snapshot).
	MetricsFilesystemSnapshotSubdir = "filesystem_snapshot"
	// MetricsCommandMetricsSubdir: day-rolled and timeseries chunks for command execution metrics.
	MetricsCommandMetricsSubdir = "command_metrics"

	// Common file names
	ProjectConfigFile = configYAMLFileName
	FeatureFlagsFile  = "feature_flags.json"
	// CLIHookProfileFile stores built-in CLI hook bindings (enable/disable, optional tray entry). See pkg/clihooks.
	CLIHookProfileFile = "cli_hook_profile.json"
	// AgentIdleStoreFile is the accumulator state file for agent idleness tracking.
	AgentIdleStoreFile = "agent_idle_store.json"
	// IdentityStatusFile is the last-resolved CLI/MCP seat snapshot (ACC, lane, roles).
	// Written by AuthMiddleware / system whoami. Not CAS.
	IdentityStatusFile = "identity_status.json"
	// ObserverTipsFile is the AST observer coach cache under StateDir.
	ObserverTipsFile = "observer_tips.json"
	// AgentChatChannelConfigFile is lite-file policy for the agent chat channel pilot (steward rules, enable/disable).
	// See pkg/datacell, DATA_CELL_RUNTIME_ORGANISM.md.
	AgentChatChannelConfigFile = "agent_chat_channel.json"
	// AgentChatChannelEventsFile is append-only JSONL for chat-channel events (same directory contract as IDE hook logs).
	AgentChatChannelEventsFile = "agent_chat_channel.jsonl"
	// AgentIdlenessFile is lite-file storage for the agent idle accumulator.
	AgentIdlenessFile = "agent_idleness.json"
	// IdeBridgeControlJSONLFile is append-only control bus for extensions/zqk-ide-bridge (zqk_ide_bridge_v1).
	IdeBridgeControlJSONLFile = "ide_bridge_control.jsonl"
	// IDEHooksLogsSubdir holds IDE hook and chat-channel JSONL logs under .zqk/logs/.
	IDEHooksLogsSubdir = "ide-hooks"
	// DataCellLogsSubdir holds data-cell stewardship JSONL under ProjectDataDir/LogsDir (steward enqueue queue; see pkg/datacell).
	DataCellLogsSubdir = "datacell"
	// StewardEnqueueJSONLFile is the append-only steward enqueue queue drained by data_cell_envelope_tick.
	StewardEnqueueJSONLFile = "steward_enqueue.jsonl"
	// DataCellRuntimeManifestFile is optional JSON declaring protocol_version for the datacell runtime layout. See pkg/datacell, DATA_CELL_RUNTIME_ORGANISM.md.
	DataCellRuntimeManifestFile = "datacell_runtime.json"
	// TrayYAMLFile is the optional tray manifest under ProjectDataDir (merged with embedded defaults). See pkg/tray, pkg/datacell.
	TrayYAMLFile        = "tray.yaml"
	ValidationCacheFile = "validation_cache.json"
	ObjectIDCacheFile   = "object-id-cache.json"
	CommandMetricsFile  = "command_metrics.json"
	// ContextEventsJSONLFile is append-only JSONL for cross-cutting context / criteria / matrix signals (pkg/contextevents).
	ContextEventsJSONLFile = "context_events.jsonl"
	SamplerConfigFile      = "sampler_config.yaml"
	MCPConfigFile          = configYAMLFileName // Under MCPDir
	// PeerSeatsFile is the local mesh seat→transport map under MeshStateSubdir.
	PeerSeatsFile = "peer_seats.json"
	// PeerAckAwaitsFile is the peer-ack await registry under MeshStateSubdir.
	PeerAckAwaitsFile       = "peer_ack_awaits.json"
	BlockingCheckConfigFile = "blocking_check_config.yaml"
	// HighVolumeKindsConfigFile is the canonical list of stream-backed kinds.
	// Located under ProcessInternalConfigsDir. Drives streamStorageEnabledKinds in pkg/storage.
	HighVolumeKindsConfigFile = "high_volume_kinds.yaml"
	KindMappingsConfigFile    = "kind_mappings_config.yaml"
	IdPrefixesConfigFile      = "id_prefixes_config.yaml"
	PathsConfigFile           = "paths_config.yaml"
	NamespacesConfigFile      = "namespaces_config.yaml"
	ScannerConfigFile         = "scanner_config.yaml"
	CommandTimeoutsConfigFile = "command_timeouts.yaml" // Under repo-root ConfigDir
	MCPLogsDir                = "logs"                  // Under MCPDir
	MCPTraceLogPrefix         = "mcp-trace-"
	LogEventsPrefix           = "log-events-"
	ComponentsLogDir          = "components" // Under LogsDir

	// AuditStreamsDir: legacy name; audit_event streams now live under StreamsDir ( .zqk/streams/audit_event/ ). Kept for migration or external reference.
	AuditStreamsDir = "audit_streams"
	// StreamsDir: generic stream storage root for high-volume kinds (e.g. change_journal_entry) under .zqk/streams/
	StreamsDir = "streams"
	// TestBundlesDir is the subdirectory under ProjectDataDir where test bundles are saved/loaded (definitions)
	TestBundlesDir = "test-bundles"

	// Scheduler-related constants
	SchedulerDir = "scheduler"
	// SchedulerConfigFile is the scheduler configuration file name
	SchedulerConfigFile = configYAMLFileName
	// SchedulerPIDFile is the scheduler PID file name
	SchedulerPIDFile = "scheduler.pid"
	// SchedulerKeepAliveFile is the scheduler keep-alive file name
	SchedulerKeepAliveFile = "scheduler.keepalive"
	// SchedulerDaemonUnhealthyFile is written by external health-check when daemon is down or keep-alive stale.
	// Monitoring can watch for this file; health-check removes it when daemon is healthy.
	SchedulerDaemonUnhealthyFile = "daemon-unhealthy.json"
	// SchedulerNoAutoRestartFile is created by "zqk scheduler stop" so ensure-scheduler-running.sh (cron) does not restart the daemon until the user runs "zqk scheduler start" again.
	SchedulerNoAutoRestartFile = "no-auto-restart"
	// SchedulerTriggersDir is the subdirectory for trigger queue files
	SchedulerTriggersDir = "triggers"
	// SchedulerTriggersQueueFile is the trigger queue file name
	SchedulerTriggersQueueFile = "queue.json"
	// SchedulerLocksDir is the subdirectory for lock files
	SchedulerLocksDir = "locks"
	// SchedulerLogsDir is the subdirectory for job execution logs (legacy name; see SchedulerJobLogsSubdir).
	SchedulerLogsDir = "logs"
	// SchedulerJobLogsSubdir is the subdirectory under ProjectDataDir/LogsDir for per-job logs.
	// Full path: .zqk/logs/scheduler/<job-id>/<job-id>.log (colocated with other runtime logs under .zqk).
	SchedulerJobLogsSubdir = "scheduler"
	// SchedulerCVSSubdir is the convergence-session / CVS log namespace under scheduler logs.
	// Full path: .zqk/logs/scheduler/cvs/ — CVS-wide JSONL (e.g. cvs_measurement_events.jsonl) and a nested test-bundles dir.
	SchedulerCVSSubdir = "cvs"
	// SchedulerTestBundlesSubdir is the subdir under SchedulerCVSSubdir for test-bundle job logs (shared folder).
	// Full path: .zqk/logs/scheduler/cvs/test-bundles/; files: events.jsonl, health.jsonl, <job-id>.stdout, bundle-*.log.
	SchedulerTestBundlesSubdir = "test-bundles"
	// SchedulerChurnRunsSubdir is the subdir for legacy/high-churn one-off job logs (SCH-<unix>-<kind>-<id> pattern).
	// One shared folder avoids hundreds of duplicate per-timestamp directories under .zqk/logs/scheduler/.
	SchedulerChurnRunsSubdir = "churn-runs"

	// Persistent project root (workspace-scoped): .zqk/current_root in the workspace (repo) root.
	// Contains one line: absolute path or path relative to workspace. See PROJECT_ROOT_USE_AND_SCHEDULER_ALIGNMENT.md.
	CurrentRootFile = "current_root"

	// BrandSettingsFilename is the per-project settings file (e.g. zqk-settings.yaml).
	// Defines project_root, staleness_check_dirs, aliases, cli.default_context. Required for scheduler and orientation.
	// See .zqk/cli/specs/schemas/brand_settings.schema.json and PATH_ALIAS_RESOLUTION.md §6.
	BrandSettingsFilename = "zqk-settings.yaml"
	// TestSettingsFilename is the settings file used when ZQK_TEST_ROOT is set. Tests load this file
	// instead of zqk-settings.yaml so project data is never touched. SetupTestEnvironment creates it.
	TestSettingsFilename = "test-settings.yaml"
	// ZqkTestSettingsFilename is an alternate test-root settings file (same schema as brand_settings).
	// Loaded when TestSettingsFilename is absent so isolated zqk-ts + ZQK_TS_TEST_ROOT projects can ship one file.
	ZqkTestSettingsFilename = "zqk-test-settings.yaml"

	// Pre-commit: all state under .zqk/pre-commit/ (category files + single results.json for the hook)
	PreCommitDir         = "pre-commit"
	PreCommitResultsFile = "results.json" // aggregated file read by hook: .zqk/pre-commit/results.json

	// Local CI directory and file constants (under .zqk/ci)
	LocalCIDir            = "ci"
	LocalCIWorkdirName    = "workdir"
	LocalCITreesDir       = "trees"
	LocalCIArchivesDir    = "archives"
	LocalCIArchiveGitDir  = "git"
	LocalCISourceSHAFile  = "SOURCE_SHA"
	LocalCIPromotedAtFile = "PROMOTED_AT"
	LocalCICurrentEnvFile = "CURRENT.env"

	// DraftsDir is under ProjectDataDir: default output for `zqk new` (editable YAML before object create).
	DraftsDir = "drafts"
	// LastDraftPointerFile records the most recently written draft path for implicit `object create` / tooling.
	LastDraftPointerFile = "last-draft.yaml"
	// ObjectDraftsDir holds id-keyed preliminary object YAML (storage draft plane), distinct from DraftsDir templates.
	// Layout: ProjectDataDir/object_drafts/<kind>/<shard>/<id>.yaml — see pkg/storage/object_draft_plane.go.
	ObjectDraftsDir = "object_drafts"

	// Process directory constants (under .zqk e.g. .zqk/process and .zqk/specs)
	ProcessDir                    = ".zqk/process"
	ProcessInternalDir            = ".zqk/specs"
	ProcessInternalConfigsDir     = ".zqk/specs/configs"
	ProcessInternalAPISpecsDir    = ".zqk/specs/api_specs"
	ProcessInternalLifecyclesDir  = ".zqk/specs/lifecycles"
	ProcessInternalObjectSpecsDir = ".zqk/specs/objects"
	// ProcessInternalPipelineOutcomeKeysFile is the declarative registry for pkg/pipeline outcome map keys (codegen).
	ProcessInternalPipelineOutcomeKeysFile = ".zqk/specs/configs/pipeline_outcome_keys.yaml"
	ProcessInternalTraitsDir               = ".zqk/specs/traits"
	ProcessInternalProfileSpecsDir         = ".zqk/specs/profile_specs"
	ProcessArchitectureDir                 = "docs/architecture"
	ProcessPoliciesDir                     = ".zqk/process/policies"
	ProcessPlanningDir                     = ".zqk/process/planning"
	ProcessAuditDir                        = ".zqk/process/audit"
	ProcessAccountsDir                     = ".zqk/process/accounts"
	ProcessPersonasDir                     = ".zqk/process/personas"
	AccountIndexFile                       = ".account.index"
	PersonaIndexFile                       = ".persona.index"
	RoleIndexFile                          = ".role.index"
	ProcessAuthStrategiesDir               = ".zqk/process/auth_strategies"
	ProcessBacklogDir                      = ".zqk/process/backlog_items"
	ProcessTestCasesDir                    = ".zqk/process/test_cases"
	ProcessKeystoreDir                     = ".zqk/process/keystore"
	ProcessRolesDir                        = ".zqk/process/roles"
	ProcessMissionsDir                     = ".zqk/process/missions"
	ProcessVisionsDir                      = ".zqk/process/visions"
	ProcessGoalsDir                        = ".zqk/process/goals"
	ProcessWorkstreamsDir                  = ".zqk/process/workstreams"
	ProcessPriorityPlansDir                = ".zqk/process/priority_plans"
	ProcessAgentSkillsDir                  = ".zqk/process/agent_skills"

	// Documentation directory constants
	OnboardingDir = "docs/onboarding"

	// CLI directory constants
	CLIProfilesDir = "pkg/cli/profiles"

	// CLI specs (under project data); used by bootstrap and command builders
	CLISpecsDir         = "cli/specs"
	PathKeyCommandSpecs = "command_specs"
	// System-health and quarantine (under project data)
	SystemHealthDir = "system-health"
	QuarantineDir   = "quarantine" // under SystemHealthDir
	AutofixDir      = "autofix"
	LockDir         = "lock"

	// Standard source directory constants
	DocsDir        = "docs"
	DocsQualityDir = DocsDir + "/quality"
	PkgDir         = "pkg"
	// ConfigPackageDir is the root config package under PkgDir (import …/pkg/config).
	ConfigPackageDir = "config"
	// SpecbuilderDir is the specbuilder tree under PkgDir (import …/pkg/specbuilder/…).
	SpecbuilderDir = "specbuilder"
	// ObjectsPackageDir is the Go package directory under PkgDir (generated field_keys.go, kinds, etc.).
	ObjectsPackageDir = "objects"
	// FieldKeysGoFile is the generated ontology field-key constants source (under PkgDir/ObjectsPackageDir).
	FieldKeysGoFile = "field_keys.go"
	CmdDir          = "cmd"
	InternalDir     = "internal"
	ToolsDir        = "tools"
	ScriptsDir      = "scripts"

	// File extension constants
	YAMLExtension        = ".yaml"
	JSONExtension        = ".json"
	MarkdownExtension    = ".md"
	MarkdownAltExtension = ".markdown"

	// Lock and temporary file suffixes (append to base path for atomic/locked writes)
	LockFileSuffix = ".lock"
	TmpFileSuffix  = ".tmp"

	// Profile constants
	ProfileExtendsNull = "null" // Value used to indicate no parent profile extension

	// Namespace ID constants
	// These define standard namespace identifiers used throughout the project
	//
	// NOTE: namespace IDs are configured dynamically for white-label support.

	// Path reference scheme prefixes: path refs use these to distinguish resolution.
	// Resolve via path alias cache at runtime before resolving any path on the local system.
	//   prefix:<alias>  → project-relative path from cache (e.g. "prefix:docs" → "<project-root>/docs")
	//   abs:<uri>       → absolute or file URI (e.g. "abs:file:///tmp/foo" or "abs:/local/abs/path")
	//   web:<url>       → web URL (e.g. "web:https://example.com/resource")
	PathSchemePrefix = "prefix:"
	PathSchemeAbs    = "abs:"
	PathSchemeWeb    = "web:"
)

// Default POSIX permissions for directories and files created under the project tree.
// Use these instead of raw octal literals so linters and code search stay consistent.
const (
	DirPerm700  fs.FileMode = 0o700 // e.g. .zqk/process/keystore (credential material)
	DirPerm750  fs.FileMode = 0o750 // e.g. scheduler health artifacts (owner + group r-x)
	DirPerm755  fs.FileMode = 0o755
	FilePerm600 fs.FileMode = 0o600
	FilePerm644 fs.FileMode = 0o644
	FilePerm755 fs.FileMode = 0o755 // e.g. executable scripts, binaries, test runners
	FilePerm700 fs.FileMode = 0o700 // e.g. private executable scripts, key material
)
