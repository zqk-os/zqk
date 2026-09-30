package zqkenv

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/brand"
)

// IsCommunityEdition is a global flag set by the community binary.
var IsCommunityEdition bool = false

const _sfxAccountID = "ACCOUNT_ID"

const _sfxStressRealRoot = "STRESS_REAL_ROOT"
const _sfxAgentPromptDeliveryHTTPBearer = "AGENT_PROMPT_DELIVERY_HTTP_BEARER" //nolint:gosec
const _sfxAgentPromptDeliveryHTTPURL = "AGENT_PROMPT_DELIVERY_HTTP_URL"
const _sfxAgentPromptKeystrokeLog = "AGENT_PROMPT_KEYSTROKE_LOG"
const _sfxAgentRulesDir = "AGENT_RULES_DIR"
const _sfxAgentWebhookSlackAllAgentFarm = "AGENT_WEBHOOK_SLACK_ALL_AGENT_FARM"
const _sfxAgentWorktreeRoot = "AGENT_WORKTREE_ROOT"
const _sfxAgentID = "AGENT_ID"
const _sfxAgentGitEmail = "AGENT_GIT_EMAIL"
const _sfxAgentGitName = "AGENT_GIT_NAME"
const _sfxAgentPubKey = "AGENT_PUB_KEY"
const _sfxAgentPrivateKey = "AGENT_PRIVATE_KEY"
const _sfxAgentSyncMaxLoops = "AGENT_SYNC_MAX_LOOPS"
const _sfxAgentMaxVerificationAttempts = "AGENT_MAX_VERIFICATION_ATTEMPTS"
const _sfxAgentSyncMaxStagnantTicks = "AGENT_SYNC_MAX_STAGNANT_TICKS"
const _sfxAPIKey = "API_KEY"
const _sfxAggregationMetricCreationTimeout = "AGGREGATION_METRIC_CREATION_TIMEOUT"
const _sfxAllowForegroundGoTest = "ALLOW_FOREGROUND_GO_TEST"
const _sfxBin = "BIN"
const _sfxBulkBenchSize = "BULK_BENCH_SIZE"
const _sfxLocalCIDir = "LOCAL_CI_DIR"
const _sfxLocalCIArchiveKeep = "LOCAL_CI_ARCHIVE_KEEP"

const _sfxCacheDiagnosticEnabled = "CACHE_DIAGNOSTIC_ENABLED"
const _sfxCacheDiagnosticObjects = "CACHE_DIAGNOSTIC_OBJECTS"
const _sfxCacheDiagnosticPrefixes = "CACHE_DIAGNOSTIC_PREFIXES"
const _sfxIDEPastePrefixSteps = "CURSOR_PASTE_PREFIX_STEPS"
const _sfxIDEPasteApp = "IDE_PASTE_APP"
const _sfxCVSPerTestLedgerMaxFailures = "CVS_LEDGER_MAX_FAILURES"
const _sfxCRUDBaselineCount = "CRUD_BASELINE_COUNT"
const _sfxDisableCriteriaAutoValidate = "DISABLE_CRITERIA_AUTO_VALIDATE"
const _sfxTelemetryOptIn = "TELEMETRY_OPT_IN"
const _sfxHostloadDisable = "HOSTLOAD_DISABLE"
const _sfxHostCPUBackpressure = "HOST_CPU_BACKPRESSURE"
const _sfxCRUDHeapProfile = "CRUD_HEAP_PROFILE"
const _sfxCRUDProfile = "CRUD_PROFILE"
const _sfxDebugValidationObjectIDs = "DEBUG_VALIDATION_OBJECT_IDS"
const _sfxDiagnosticsDir = "DIAGNOSTICS_DIR"
const _sfxDiagnosticsThreadStackSkipMin = "DIAGNOSTICS_THREAD_STACK_SKIP_MIN_GOROUTINES"
const _sfxEnableBootstrapCRUDTests = "ENABLE_BOOTSTRAP_CRUD_TESTS"
const _sfxEnableSpecCellIntegrationTests = "ENABLE_SPEC_CELL_INTEGRATION_TESTS"
const _sfxEnableSpecCellREQ019Validate = "ENABLE_SPEC_CELL_REQ019_VALIDATE"
const _sfxEnableCASMigrationScenarioTest = "ENABLE_CAS_MIGRATION_SCENARIO_TEST"
const _sfxEnableCLIScenarioTests = "ENABLE_CLI_SCENARIO_TESTS"
const _sfxEditorProfile = "EDITOR_PROFILE"
const _sfxEnableHashRegistryCoordinatorTests = "ENABLE_HASH_REGISTRY_COORDINATOR_TESTS"
const _sfxEnableMigrateLegacyToStreamIntegrationTest = "ENABLE_MIGRATE_LEGACY_TO_STREAM_INTEGRATION_TEST"
const _sfxEnableIOQueueShutdownTests = "ENABLE_IOQUEUE_SHUTDOWN_TESTS"
const _sfxEnablePublicCandidateTest = "ENABLE_PUBLIC_CANDIDATE_TEST"
const _sfxPublicCandidateDir = "PUBLIC_CANDIDATE_DIR"
const _sfxPublicCandidateAllowClobber = "PUBLIC_CANDIDATE_ALLOW_CLOBBER"
const _sfxExecSource = "EXEC_SOURCE"
const _sfxGraphDatabase = "GRAPH_DATABASE"
const _sfxGraphEnabled = "GRAPH_ENABLED"
const _sfxAdminGraphEnabled = "ADMIN_GRAPH_ENABLED"
const _sfxGraphHost = "GRAPH_HOST"
const _sfxGraphPassword = "GRAPH_PASSWORD"
const _sfxGraphPoolSize = "GRAPH_POOL_SIZE"
const _sfxGraphPort = "GRAPH_PORT"
const _sfxGraphUsername = "GRAPH_USERNAME"
const _sfxMockGraph = "MOCK_GRAPH"
const _sfxAdminMockGraph = "ADMIN_MOCK_GRAPH"
const _sfxFFMPEGWorkerEndpoint = "FFMPEG_WORKER_ENDPOINT"
const _sfxHighVolumeCacheBuildWorkers = "HIGH_VOLUME_CACHE_BUILD_WORKERS"
const _sfxInTest = "IN_TEST"
const _sfxHighVolumeCacheKindParallelism = "HIGH_VOLUME_CACHE_KIND_PARALLELISM"
const _sfxJobID = "JOB_ID"

// Historical alias set by run-tests-bg.sh / older scheduler wrappers (see agent_guard).
const _sfxSchedulerJobID = "SCHEDULER_JOB_ID"
const _sfxListCountMaxConcurrent = "LIST_COUNT_MAX_CONCURRENT"
const _sfxListReadWorkers = "LIST_READ_WORKERS"
const _sfxMaxOSThreads = "MAX_OS_THREADS"
const _sfxMaxObjectYAMLIO = "MAX_OBJECT_YAML_IO"
const _sfxLLMAPIKey = "LLM_API_KEY" //nolint:gosec
const _sfxLLMBaseURL = "LLM_BASE_URL"
const _sfxLLMChatModel = "LLM_CHAT_MODEL"
const _sfxLLMContextWindowSize = "LLM_CONTEXT_WINDOW_SIZE"
const _sfxSwarmMaxSteps = "SWARM_MAX_STEPS"
const _sfxLLMEmbedModel = "LLM_EMBED_MODEL"
const _sfxLLMTrace = "LLM_TRACE"
const _sfxLLMTraceDir = "LLM_TRACE_DIR"
const _sfxLLMTemperature = "LLM_TEMPERATURE"
const _sfxLLMTimeout = "LLM_TIMEOUT"
const _sfxLLMTopP = "LLM_TOP_P"
const _sfxQwenBaseURL = "QWEN_BASE_URL"
const _sfxQwenAPIKey = "QWEN_API_KEY" //nolint:gosec
const _sfxMetricsChunkRetentionDays = "METRICS_CHUNK_RETENTION_DAYS"
const _sfxMCPAccountID = "MCP_ACCOUNT_ID"
const _sfxMCPConfigPath = "MCP_CONFIG_PATH"
const _sfxMCPKeystoreKeyID = "MCP_KEYSTORE_KEY_ID"
const _sfxMCPExternalSecretKey = "MCP_EXTERNAL_SECRET_KEY" //nolint:gosec
const _sfxMCPPermissions = "MCP_PERMISSIONS"
const _sfxMCPRoles = "MCP_ROLES"
const _sfxMCPTimeout = "MCP_TIMEOUT"
const _sfxMCPTrace = "MCP_TRACE"
const _sfxMCPTraceFile = "MCP_TRACE_FILE"
const _sfxURNBrand = "URN_BRAND"
const _sfxSchedulerPackageConcurrencyMaxWait = "SCHEDULER_PACKAGE_CONCURRENCY_MAX_WAIT"
const _sfxFileutilMetrics = "FILEUTIL_METRICS"
const _sfxSession = "SESSION"
const _sfxParentPID = "PARENT_PID"
const _sfxPersona = "PERSONA"
const _sfxPopulateScenarioTest = "POPULATE_SCENARIO_TEST"
const _sfxPreconditionsFailOpen = "PRECONDITIONS_FAIL_OPEN"
const _sfxPprof = "PPROF"
const _sfxPprofPort = "PPROF_PORT"
const _sfxProjectRoot = "PROJECT_ROOT"
const _sfxProductionKeystoreStrict = "PRODUCTION_KEYSTORE_STRICT"
const _sfxRetentionToleranceConfig = "RETENTION_TOLERANCE_CONFIG"
const _sfxRollbackCaptureDisabled = "ROLLBACK_CAPTURE_DISABLED"
const _sfxRollbackRetainCount = "ROLLBACK_RETAIN_COUNT"
const _sfxRollbackRetainDuration = "ROLLBACK_RETAIN_DURATION"
const _sfxRoot = "ROOT"
const _sfxRunPerfTests = "RUN_PERF_TESTS"
const _sfxSchedulerAdmissionTimeout = "SCHEDULER_ADMISSION_TIMEOUT"
const _sfxSchedulerDaemonBin = "SCHEDULER_DAEMON_BIN"
const _sfxSchedulerDefaultPackageConcurrency = "SCHEDULER_DEFAULT_PACKAGE_CONCURRENCY"
const _sfxSchedulerDispatchResourceWaitMax = "SCHEDULER_DISPATCH_RESOURCE_WAIT_MAX"
const _sfxSchedulerGoroutineCap = "SCHEDULER_GOROUTINE_CAP"
const _sfxSchedulerImmediateLoadBatchPause = "SCHEDULER_IMMEDIATE_LOAD_BATCH_PAUSE"
const _sfxSchedulerImmediateLoadBatchSize = "SCHEDULER_IMMEDIATE_LOAD_BATCH_SIZE"
const _sfxSchedulerLogsConfig = "SCHEDULER_LOGS_CONFIG"
const _sfxSchedulerMaintenanceConfig = "SCHEDULER_MAINTENANCE_CONFIG"
const _sfxSchedulerMaxWallDuration = "SCHEDULER_MAX_WALL_DURATION"
const _sfxSchedulerTriggeredPoolSize = "SCHEDULER_TRIGGERED_POOL_SIZE"
const _sfxSessionID = "SESSION_ID"
const _sfxSkipDeleteAudit = "SKIP_DELETE_AUDIT"
const _sfxSkipSpecSchemaValidation = "SKIP_SPEC_SCHEMA_VALIDATION"
const _sfxSpecializationTier = "SPECIALIZATION_TIER"
const _sfxStableBinaryPath = "STABLE_BINARY_PATH"
const _sfxStorageMode = "STORAGE_MODE"
const _sfxStorageModeHybridLegacy = "STORAGE_MODE_HYBRID_LEGACY"
const _sfxProjectionFailClosed = "PROJECTION_FAIL_CLOSED"
const _sfxStreamDeltaFieldsConfig = "STREAM_DELTA_FIELDS_CONFIG"
const _sfxStreamStorageEnabled = "STREAM_STORAGE_ENABLED"
const _sfxTaskArtifacts = "TASK_ARTIFACTS"
const _sfxTableMaxRows = "TABLE_MAX_ROWS"
const _sfxSharedTestBin = "SHARED_TEST_BIN"
const _sfxTestCLIBinary = "TEST_CLI_BINARY"
const _sfxTestDataDir = "TEST_DATA_DIR"
const _sfxTestMetricsRecording = "TEST_METRICS_RECORDING"
const _sfxTestMode = "TEST_MODE"
const _sfxTestRoot = "TEST_ROOT"
const _sfxTestVerbose = "TEST_VERBOSE"
const _sfxUpdateHelpGolden = "UPDATE_HELP_GOLDEN"

// AccountID returns the environment variable name for ACCOUNT_ID (brand-prefixed).
func AccountID() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAccountID)} }

// StressRealRoot returns the env var for STRESS_REAL_ROOT (brand-prefixed).
// When set to "1", optional stress tests may use the live checkout root.
func StressRealRoot() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxStressRealRoot)} }

// APIKey returns the environment variable name for API_KEY (brand-prefixed).
func APIKey() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAPIKey)} }

// AgentPromptKeystrokeLog returns the environment variable name for AGENT_PROMPT_KEYSTROKE_LOG (brand-prefixed).
// Passed to osascript for optional AppleScript consumers (path to agent_prompt_ide_keystrokes.log).
func AgentPromptKeystrokeLog() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAgentPromptKeystrokeLog)} }

// AgentPromptDeliveryHTTPURL returns the environment variable name for AGENT_PROMPT_DELIVERY_HTTP_URL (brand-prefixed).
// Optional webhook URL for agent-prompt markdown POST (see pkg/agentdelivery.HTTPDeliverer).
func AgentPromptDeliveryHTTPURL() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxAgentPromptDeliveryHTTPURL)}
}

// AgentPromptDeliveryHTTPBearer returns the environment variable name for AGENT_PROMPT_DELIVERY_HTTP_BEARER (brand-prefixed).
// Optional Bearer token for Authorization when posting agent-prompt markdown to HTTP.
func AgentPromptDeliveryHTTPBearer() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxAgentPromptDeliveryHTTPBearer)}
}

// AgentRulesDir returns the environment variable name for AGENT_RULES_DIR (brand-prefixed).
// Optional path relative to project root for tracked *.mdc rule files (default .ide/rules when unset).
// Used by zqk system validate-agent-rules; prefer no env when using the repo default layout.
func AgentRulesDir() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAgentRulesDir)} }

// AgentWorktreeRoot returns the env var name for AGENT_WORKTREE_ROOT (brand-prefixed).
// Optional absolute base for isolated agent git worktrees. Must not be under the
// studio project root. Default when unset: $TMPDIR/zqk-worktrees/<repo-key>/.
// Kernel: POL-AGENT-WORKTREE-ISOLATION-001.
func AgentWorktreeRoot() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAgentWorktreeRoot)} }

// AgentGitName returns AGENT_GIT_NAME (brand-prefixed). Git author/committer
// name for swarm worktree commits. Default when unset: "ZQK Swarm Agent".
func AgentGitName() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAgentGitName)} }

// AgentGitEmail returns AGENT_GIT_EMAIL (brand-prefixed). Git author/committer
// email for swarm worktree commits. Default when unset: "swarm@zqk.internal".
func AgentGitEmail() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAgentGitEmail)} }

// AgentID returns the environment variable name for AGENT_ID (brand-prefixed).
func AgentID() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAgentID)} }

// AgentSyncMaxLoops returns the env var for AGENT_SYNC_MAX_LOOPS (brand-prefixed).
// Sync-loop outer poll guard; default 100. TRACK
func AgentSyncMaxLoops() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAgentSyncMaxLoops)} }

// LocalCIDir returns the environment variable name for LOCAL_CI_DIR (brand-prefixed).
func LocalCIDir() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxLocalCIDir)} }

// LocalCIArchiveKeep returns the environment variable name for LOCAL_CI_ARCHIVE_KEEP (brand-prefixed).
func LocalCIArchiveKeep() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxLocalCIArchiveKeep)} }

// AgentMaxVerificationAttempts returns AGENT_MAX_VERIFICATION_ATTEMPTS (brand-prefixed).
// Per-step verification retry cap; default 3. TRACK
func AgentMaxVerificationAttempts() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxAgentMaxVerificationAttempts)}
}

// AgentSyncMaxStagnantTicks returns AGENT_SYNC_MAX_STAGNANT_TICKS (brand-prefixed).
// Abort when task progress fingerprint is unchanged this many ticks; default 10.
// TRACK: follow-up in kernel backlog
func AgentSyncMaxStagnantTicks() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxAgentSyncMaxStagnantTicks)}
}

// AgentPubKey returns the environment variable name for AGENT_PUB_KEY (brand-prefixed).
func AgentPubKey() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAgentPubKey)} }

// AgentPrivateKey returns the environment variable name for AGENT_PRIVATE_KEY (brand-prefixed).
func AgentPrivateKey() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAgentPrivateKey)} }

// AggregationMetricCreationTimeout returns the environment variable name for AGGREGATION_METRIC_CREATION_TIMEOUT (brand-prefixed).
func AggregationMetricCreationTimeout() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxAggregationMetricCreationTimeout)}
}

// Bin returns the environment variable name for BIN (brand-prefixed).
func Bin() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxBin)} }

// BulkBenchSize returns the environment variable name for BULK_BENCH_SIZE (brand-prefixed).
func BulkBenchSize() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxBulkBenchSize)} }

// CacheDiagnosticEnabled returns the environment variable name for CACHE_DIAGNOSTIC_ENABLED (brand-prefixed).
func CacheDiagnosticEnabled() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxCacheDiagnosticEnabled)} }

// CacheDiagnosticObjects returns the environment variable name for CACHE_DIAGNOSTIC_OBJECTS (brand-prefixed).
func CacheDiagnosticObjects() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxCacheDiagnosticObjects)} }

// CacheDiagnosticPrefixes returns the environment variable name for CACHE_DIAGNOSTIC_PREFIXES (brand-prefixed).
func CacheDiagnosticPrefixes() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxCacheDiagnosticPrefixes)} }

// IDEPastePrefixSteps returns the environment variable name for CURSOR_PASTE_PREFIX_STEPS (brand-prefixed).
// Comma-separated keystroke tokens before ⌘V in IDE paste automation (escape, cmd_l, option_cmd_e).
// See cmd/zqk/scheduler/ide_paste_automation.go and scripts/ide/README.md.
func IDEPastePrefixSteps() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxIDEPastePrefixSteps)} }

// IDEPasteApp returns the environment variable name for IDE_PASTE_APP (brand-prefixed).
// macOS application / System Events process name targeted by paste AppleScript (default Cursor).
func IDEPasteApp() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxIDEPasteApp)} }

// CVSPerTestLedgerMaxFailures returns the env var name for CVS_LEDGER_MAX_FAILURES (brand-prefixed).
// Caps how many parsed failed tests are recorded under convergence_session.after_state_snapshot.per_test_failure_ledger_v1.
// Omit or invalid values use the scheduler default; set to 0 for no limit (skip threshold).
func CVSPerTestLedgerMaxFailures() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxCVSPerTestLedgerMaxFailures)}
}

// CRUDBaselineCount returns the environment variable name for CRUD_BASELINE_COUNT (brand-prefixed).
func CRUDBaselineCount() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxCRUDBaselineCount)} }

// DisableCriteriaAutoValidate returns the environment variable name for DISABLE_CRITERIA_AUTO_VALIDATE (brand-prefixed).
// When set to 1/true, the scheduler does not apply criteria status=validated from green SCH-run-* test bundles
// (criteria_verification_satisfied). Unset: allow auto-apply (opt-out).
func DisableCriteriaAutoValidate() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxDisableCriteriaAutoValidate)}
}

// TelemetryOptIn returns the environment variable name for TELEMETRY_OPT_IN (brand-prefixed).
func TelemetryOptIn() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxTelemetryOptIn)}
}

// HostloadDisable returns the env name for HOSTLOAD_DISABLE (brand-prefixed).
// When set to 1, scheduler and validation fan-out ignore live host CPU pressure
// (always treat the host as having headroom). TRACK: TDE-CEF-HOST-CPU-BACKPRESSURE-001
func HostloadDisable() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxHostloadDisable)} }

// HostCPUBackpressure returns the environment variable name for HOST_CPU_BACKPRESSURE (brand-prefixed).
func HostCPUBackpressure() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxHostCPUBackpressure)} }

// CRUDHeapProfile returns the environment variable name for CRUD_HEAP_PROFILE (brand-prefixed).
func CRUDHeapProfile() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxCRUDHeapProfile)} }

// CRUDProfile returns the environment variable name for CRUD_PROFILE (brand-prefixed).
func CRUDProfile() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxCRUDProfile)} }

// DebugValidationObjectIDs returns the environment variable name for DEBUG_VALIDATION_OBJECT_IDS (brand-prefixed).
func DebugValidationObjectIDs() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxDebugValidationObjectIDs)}
}

// DiagnosticsDir returns the environment variable name for DIAGNOSTICS_DIR (brand-prefixed).
func DiagnosticsDir() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxDiagnosticsDir)} }

// DiagnosticsThreadStackSkipMinGoroutines returns the environment variable name for DIAGNOSTICS_THREAD_STACK_SKIP_MIN_GOROUTINES (brand-prefixed).
// CaptureDiagnostics uses this threshold to skip redundant runtime.Stack output in *_threads.txt when NumGoroutine() is at or above this value.
// Positive integer: skip when goroutine count >= value. Zero or negative: do not skip on threshold (always attempt runtime.Stack).
// Unset: use the package default (see pkg/diagnostics threadDumpSkipRuntimeStackMinGoroutines).
func DiagnosticsThreadStackSkipMinGoroutines() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxDiagnosticsThreadStackSkipMin)}
}

// EnableBootstrapCRUDTests returns the environment variable name for ENABLE_BOOTSTRAP_CRUD_TESTS (brand-prefixed).
func EnableBootstrapCRUDTests() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxEnableBootstrapCRUDTests)}
}

// EnableSpecCellIntegrationTests returns the environment variable name for ENABLE_SPEC_CELL_INTEGRATION_TESTS (brand-prefixed).
// When set to 1/true/yes, runs integration tests for greenfield init, bootstrap specs, spec index, and update-specs (data cell / alpha launch gate suite).
func EnableSpecCellIntegrationTests() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxEnableSpecCellIntegrationTests)}
}

// EnableSpecCellREQ019Validate returns the environment variable name for ENABLE_SPEC_CELL_REQ019_VALIDATE (brand-prefixed).
// When set together with [EnableSpecCellIntegrationTests], field define runs update-specs with --validate (REQ-019 / LoadSpecAndValidate).
// Default off: bundled bootstrap specs still carry inherited-field checklist debt, so full-spec validation fails until that debt is cleared.
func EnableSpecCellREQ019Validate() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxEnableSpecCellREQ019Validate)}
}

// EnableCASMigrationScenarioTest returns the environment variable name for ENABLE_CAS_MIGRATION_SCENARIO_TEST (brand-prefixed).
func EnableCASMigrationScenarioTest() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxEnableCASMigrationScenarioTest)}
}

// EnableCLIScenarioTests returns the environment variable name for ENABLE_CLI_SCENARIO_TESTS (brand-prefixed).
func EnableCLIScenarioTests() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxEnableCLIScenarioTests)} }

// EditorProfile returns the environment variable name for EDITOR_PROFILE (brand-prefixed).
// Configures the default experience profile for interactive TUIs: "newb", "pro", "jedi".
func EditorProfile() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxEditorProfile)} }

// EnableHashRegistryCoordinatorTests returns the environment variable name for ENABLE_HASH_REGISTRY_COORDINATOR_TESTS (brand-prefixed).
func EnableHashRegistryCoordinatorTests() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxEnableHashRegistryCoordinatorTests)}
}

// EnableIOQueueShutdownTests returns the environment variable name for ENABLE_IOQUEUE_SHUTDOWN_TESTS (brand-prefixed).
func EnableIOQueueShutdownTests() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxEnableIOQueueShutdownTests)}
}

// EnablePublicCandidateTest returns the environment variable name for ENABLE_PUBLIC_CANDIDATE_TEST (brand-prefixed).
// When set to 1, runs the open-core public-candidate sync/police integration test.
func EnablePublicCandidateTest() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxEnablePublicCandidateTest)}
}

// PublicCandidateDir is the disposable export dest for sync-public-candidate.sh.
// Never the live TPM checkout (zqk-public-candidate). TRACK
func PublicCandidateDir() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxPublicCandidateDir)} }

// PublicCandidateAllowClobber is human break-glass to rm -rf the well-known product sibling.
func PublicCandidateAllowClobber() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxPublicCandidateAllowClobber)}
}

// AgentWebhookSlackAllAgentFarm returns the environment variable for AGENT_WEBHOOK_SLACK_ALL_AGENT_FARM (brand-prefixed).
func AgentWebhookSlackAllAgentFarm() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxAgentWebhookSlackAllAgentFarm)}
}

// EnableMigrateLegacyToStreamIntegrationTest returns the environment variable name for ENABLE_MIGRATE_LEGACY_TO_STREAM_INTEGRATION_TEST (brand-prefixed).
// When set to 1/true/yes, runs cmd/zqk/system.TestMigrateLegacyToStream_subprocessIntegration (greenfield init, legacy audit_event YAML, subprocess migrate-legacy-to-stream).
func EnableMigrateLegacyToStreamIntegrationTest() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxEnableMigrateLegacyToStreamIntegrationTest)}
}

// GraphDatabase returns the environment variable name for GRAPH_DATABASE (brand-prefixed).
func GraphDatabase() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxGraphDatabase)} }

// GraphEnabled returns the environment variable name for GRAPH_ENABLED (brand-prefixed).
func GraphEnabled() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxGraphEnabled)} }

// AdminGraphEnabled returns the environment variable name for ADMIN_GRAPH_ENABLED (brand-prefixed).
func AdminGraphEnabled() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAdminGraphEnabled)} }

// GraphHost returns the environment variable name for GRAPH_HOST (brand-prefixed).
func GraphHost() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxGraphHost)} }

// GraphPassword returns the environment variable name for GRAPH_PASSWORD (brand-prefixed).
func GraphPassword() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxGraphPassword)} }

// GraphPoolSize returns the environment variable name for GRAPH_POOL_SIZE (brand-prefixed).
func GraphPoolSize() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxGraphPoolSize)} }

// GraphPort returns the environment variable name for GRAPH_PORT (brand-prefixed).
func GraphPort() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxGraphPort)} }

// GraphUsername returns the environment variable name for GRAPH_USERNAME (brand-prefixed).
func GraphUsername() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxGraphUsername)} }

// MockGraph returns the environment variable name for MOCK_GRAPH (brand-prefixed).
func MockGraph() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxMockGraph)} }

// AdminMockGraph returns the environment variable name for ADMIN_MOCK_GRAPH (brand-prefixed).
func AdminMockGraph() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAdminMockGraph)} }

// LegacyMockGraph returns the unprefixed MOCK_GRAPH key that graph validation still honors alongside
// the brand-prefixed one. Exposed so callers do not restate the bare literal.
func LegacyMockGraph() EnvVar { return EnvVar{Key: _sfxMockGraph} }

// FFMPEGWorkerEndpoint returns the environment variable name for FFMPEG_WORKER_ENDPOINT (brand-prefixed).
func FFMPEGWorkerEndpoint() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxFFMPEGWorkerEndpoint)} }

// HighVolumeCacheBuildWorkers returns the environment variable name for HIGH_VOLUME_CACHE_BUILD_WORKERS (brand-prefixed).
func HighVolumeCacheBuildWorkers() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxHighVolumeCacheBuildWorkers)}
}

// HighVolumeCacheKindParallelism returns the environment variable name for HIGH_VOLUME_CACHE_KIND_PARALLELISM (brand-prefixed).
// Bounds concurrent high-volume kinds during cache build (each kind still uses parallel index workers).
func HighVolumeCacheKindParallelism() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxHighVolumeCacheKindParallelism)}
}

// InTest returns the environment variable name for IN_TEST (brand-prefixed).
// When set to "true", indicates the process is running inside a test harness.
func InTest() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxInTest)} }

// IsTestBinaryPath checks whether an executable path looks like a test runner binary.
func IsTestBinaryPath(execPath string) bool {
	if execPath == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(execPath))
	return strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".test.exe")
}

// IsInTest returns true if the IN_TEST environment variable is set to "true",
// or if the process is detected to be running under a test runner.
func IsInTest() bool {
	if os.Getenv(brand.EnvVar(_sfxInTest)) == "true" {
		return true
	}
	// Check if running under go test
	if testing.Testing() || flag.Lookup("test.v") != nil {
		return true
	}
	// Fallback to checking executable name for test binary suffixes
	if len(os.Args) > 0 {
		return IsTestBinaryPath(os.Args[0])
	}
	return false
}

// JobID returns the environment variable name for JOB_ID (brand-prefixed).
func JobID() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxJobID)} }

// SchedulerJobID returns the environment variable name for SCHEDULER_JOB_ID (brand-prefixed).
// Legacy name used by run-tests-bg.sh; prefer JobID() for new code.
func SchedulerJobID() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxSchedulerJobID)} }

// Persona returns the environment variable name for PERSONA (brand-prefixed).
func Persona() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxPersona)} }

// ListCountMaxConcurrent returns the environment variable name for LIST_COUNT_MAX_CONCURRENT (brand-prefixed).
func ListCountMaxConcurrent() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxListCountMaxConcurrent)} }

// ListReadWorkers returns the environment variable name for LIST_READ_WORKERS (brand-prefixed).
func ListReadWorkers() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxListReadWorkers)} }

// MaxOSThreads returns the environment variable name for MAX_OS_THREADS (brand-prefixed).
func MaxOSThreads() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxMaxOSThreads)} }

// MaxObjectYAMLIO returns the environment variable name for MAX_OBJECT_YAML_IO (brand-prefixed).
func MaxObjectYAMLIO() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxMaxObjectYAMLIO)} }

func QwenBaseURL() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxQwenBaseURL)} }
func QwenAPIKey() EnvVar  { return EnvVar{Key: brand.EnvVar(_sfxQwenAPIKey)} }

// SwarmMaxSteps returns the environment variable name for SWARM_MAX_STEPS (brand-prefixed).
func SwarmMaxSteps() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxSwarmMaxSteps)} }

// LLMAPIKey returns the environment variable name for LLM_API_KEY (brand-prefixed).
func LLMAPIKey() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxLLMAPIKey)} }

// LLMBaseURL returns the environment variable name for LLM_BASE_URL (brand-prefixed).
func LLMBaseURL() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxLLMBaseURL)} }

// LLMChatModel returns the environment variable name for LLM_CHAT_MODEL (brand-prefixed).
func LLMChatModel() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxLLMChatModel)} }

// LLMContextWindowSize returns the environment variable name for LLM_CONTEXT_WINDOW_SIZE (brand-prefixed).
func LLMContextWindowSize() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxLLMContextWindowSize)} }

// LLMEmbedModel returns the environment variable name for LLM_EMBED_MODEL (brand-prefixed).
func LLMEmbedModel() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxLLMEmbedModel)} }

// LLMTrace returns the environment variable name for LLM_TRACE (brand-prefixed).
// Unset in a real process enables swarm prompt/response traces; "0" disables;
// "1" forces on (including under go test when a dir is also set).
func LLMTrace() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxLLMTrace)} }

// LLMTraceDir returns the environment variable name for LLM_TRACE_DIR (brand-prefixed).
// When set, swarm writes llm-trace-<engineID>.log there. Otherwise DiagnosticsDir
// or <project>/.zqk/logs/llm.
func LLMTraceDir() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxLLMTraceDir)} }

// LLMTemperature returns the environment variable name for LLM_TEMPERATURE (brand-prefixed).
// Optional float sent as OpenAI temperature when set. Empty omits the field.
func LLMTemperature() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxLLMTemperature)} }

// LLMTopP returns the environment variable name for LLM_TOP_P (brand-prefixed).
// Optional float sent as OpenAI top_p when set. Empty omits the field.
func LLMTopP() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxLLMTopP)} }

// LLMTimeout returns the environment variable name for LLM_TIMEOUT (brand-prefixed).
func LLMTimeout() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxLLMTimeout)} }

// MCPTimeout returns the environment variable name for MCP_TIMEOUT (brand-prefixed).
func MCPTimeout() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxMCPTimeout)} }

// URNBrand returns the environment variable name for URN_BRAND (brand-prefixed).
func URNBrand() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxURNBrand)} }

// SchedulerPackageConcurrencyMaxWait returns the environment variable name for SCHEDULER_PACKAGE_CONCURRENCY_MAX_WAIT (brand-prefixed).
func SchedulerPackageConcurrencyMaxWait() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxSchedulerPackageConcurrencyMaxWait)}
}

// FileutilMetrics returns the environment variable name for FILEUTIL_METRICS (brand-prefixed).
func FileutilMetrics() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxFileutilMetrics)} }

// Session returns the environment variable name for SESSION (brand-prefixed).
func Session() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxSession)} }

// MetricsChunkRetentionDays returns the environment variable name for METRICS_CHUNK_RETENTION_DAYS (brand-prefixed).
// When set to a positive integer, object-count-report prunes .zqk/metrics/{object_volume,stream_volume,filesystem_snapshot}/*.chunk older than this many days. Used by scheduled runs and CLI.
func MetricsChunkRetentionDays() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxMetricsChunkRetentionDays)}
}

// MCPAccountID returns the environment variable name for MCP_ACCOUNT_ID (brand-prefixed).
func MCPAccountID() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxMCPAccountID)} }

// ExecSource returns the environment variable name for EXEC_SOURCE (brand-prefixed, e.g. ZQK_EXEC_SOURCE).
func ExecSource() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxExecSource)} }

// MCPConfigPath returns the environment variable name for MCP_CONFIG_PATH (brand-prefixed).
func MCPConfigPath() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxMCPConfigPath)} }

// MCPKeystoreKeyID returns the environment variable name for MCP_KEYSTORE_KEY_ID (brand-prefixed).
func MCPKeystoreKeyID() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxMCPKeystoreKeyID)} }

// MCPPermissions returns the environment variable name for MCP_PERMISSIONS (brand-prefixed).
func MCPExternalSecretKey() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxMCPExternalSecretKey)} }

func MCPPermissions() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxMCPPermissions)} }

// MCPRoles returns the environment variable name for MCP_ROLES (brand-prefixed).
func MCPRoles() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxMCPRoles)} }

// MCPTrace returns the environment variable name for MCP_TRACE (brand-prefixed).
func MCPTrace() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxMCPTrace)} }

// MCPTraceFile returns the environment variable name for MCP_TRACE_FILE (brand-prefixed).
func MCPTraceFile() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxMCPTraceFile)} }

// ParentPID returns the environment variable name for PARENT_PID (brand-prefixed).
func ParentPID() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxParentPID)} }

// PopulateScenarioTest returns the environment variable name for POPULATE_SCENARIO_TEST (brand-prefixed).
func PopulateScenarioTest() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxPopulateScenarioTest)} }

// PreconditionsFailOpen returns the environment variable name for PRECONDITIONS_FAIL_OPEN (brand-prefixed).
func PreconditionsFailOpen() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxPreconditionsFailOpen)} }

// Pprof returns the environment variable name for PPROF (brand-prefixed).
func Pprof() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxPprof)} }

// PprofPort returns the environment variable name for PPROF_PORT (brand-prefixed).
func PprofPort() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxPprofPort)} }

// ProjectRoot returns the environment variable name for PROJECT_ROOT (brand-prefixed).
func ProjectRoot() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxProjectRoot)} }

// ProductionKeystoreStrict returns the environment variable name for PRODUCTION_KEYSTORE_STRICT (brand-prefixed).
// When set to "1", "true", or "yes", AuditorGate disallows falling back to local disk-based private keys
// if the trusted auditor key is absent from CAS.
func ProductionKeystoreStrict() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxProductionKeystoreStrict)}
}

// RetentionToleranceConfig returns the environment variable name for RETENTION_TOLERANCE_CONFIG (brand-prefixed).
func RetentionToleranceConfig() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxRetentionToleranceConfig)}
}

// RollbackCaptureDisabled returns the environment variable name for ROLLBACK_CAPTURE_DISABLED (brand-prefixed).
func RollbackCaptureDisabled() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxRollbackCaptureDisabled)} }

// RollbackRetainCount returns the environment variable name for ROLLBACK_RETAIN_COUNT (brand-prefixed).
func RollbackRetainCount() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxRollbackRetainCount)} }

// RollbackRetainDuration returns the environment variable name for ROLLBACK_RETAIN_DURATION (brand-prefixed).
func RollbackRetainDuration() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxRollbackRetainDuration)} }

// Root returns the environment variable name for ROOT (brand-prefixed).
func Root() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxRoot)} }

// RunPerfTests returns the environment variable name for RUN_PERF_TESTS (brand-prefixed).
func RunPerfTests() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxRunPerfTests)} }

// SchedulerAdmissionTimeout returns the environment variable name for SCHEDULER_ADMISSION_TIMEOUT (brand-prefixed).
// Duration string for [time.ParseDuration] (e.g. "60s", "2m"); how long a callback-bearing CLI one-shot
// may wait to start before callback_on_error fires and the job is disabled.
func SchedulerAdmissionTimeout() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxSchedulerAdmissionTimeout)}
}

// SchedulerDaemonBin returns the environment variable name for SCHEDULER_DAEMON_BIN (brand-prefixed).
func SchedulerDaemonBin() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxSchedulerDaemonBin)} }

// SchedulerDefaultPackageConcurrency returns the environment variable name for SCHEDULER_DEFAULT_PACKAGE_CONCURRENCY (brand-prefixed).
func SchedulerDefaultPackageConcurrency() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxSchedulerDefaultPackageConcurrency)}
}

// SchedulerDispatchResourceWaitMax returns the environment variable name for SCHEDULER_DISPATCH_RESOURCE_WAIT_MAX (brand-prefixed).
func SchedulerDispatchResourceWaitMax() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxSchedulerDispatchResourceWaitMax)}
}

// SchedulerGoroutineCap returns the environment variable name for SCHEDULER_GOROUTINE_CAP (brand-prefixed).
func SchedulerGoroutineCap() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxSchedulerGoroutineCap)} }

// SchedulerImmediateLoadBatchPause returns the environment variable name for SCHEDULER_IMMEDIATE_LOAD_BATCH_PAUSE (brand-prefixed).
// Optional duration between batches when immediate-load chunking is enabled (see SchedulerImmediateLoadBatchSize).
func SchedulerImmediateLoadBatchPause() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxSchedulerImmediateLoadBatchPause)}
}

// SchedulerImmediateLoadBatchSize returns the environment variable name for SCHEDULER_IMMEDIATE_LOAD_BATCH_SIZE (brand-prefixed).
// When > 0, initial/reload scheduling of immediate jobs runs in chunks of this size with optional pause between chunks.
func SchedulerImmediateLoadBatchSize() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxSchedulerImmediateLoadBatchSize)}
}

// SchedulerLogsConfig returns the environment variable name for SCHEDULER_LOGS_CONFIG (brand-prefixed).
func SchedulerLogsConfig() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxSchedulerLogsConfig)} }

// SchedulerMaintenanceConfig returns the environment variable name for SCHEDULER_MAINTENANCE_CONFIG (brand-prefixed).
func SchedulerMaintenanceConfig() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxSchedulerMaintenanceConfig)}
}

// SchedulerMaxWallDuration returns the environment variable name for SCHEDULER_MAX_WALL_DURATION (brand-prefixed).
// Optional duration string for [time.ParseDuration] (e.g. "45m", "2h"); when set, the scheduler daemon cancels
// its main context after this wall time from process start (graceful shutdown). Unset: no wall limit.
func SchedulerMaxWallDuration() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxSchedulerMaxWallDuration)}
}

// SchedulerTriggeredPoolSize returns the environment variable name for SCHEDULER_TRIGGERED_POOL_SIZE (brand-prefixed).
func SchedulerTriggeredPoolSize() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxSchedulerTriggeredPoolSize)}
}

// SessionID returns the environment variable name for SESSION_ID (brand-prefixed).
func SessionID() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxSessionID)} }

// SkipDeleteAudit returns the environment variable name for SKIP_DELETE_AUDIT (brand-prefixed).
func SkipDeleteAudit() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxSkipDeleteAudit)} }

// SkipSpecSchemaValidation returns the environment variable name for SKIP_SPEC_SCHEMA_VALIDATION (brand-prefixed).
func SkipSpecSchemaValidation() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxSkipSpecSchemaValidation)}
}

// SpecializationTier returns the environment variable name for SPECIALIZATION_TIER (brand-prefixed).
// Defines the cellular specialization of the node (neuron, muscle, heart, lung).
func SpecializationTier() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxSpecializationTier)} }

// StableBinaryPath returns the environment variable name for STABLE_BINARY_PATH (brand-prefixed).
func StableBinaryPath() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxStableBinaryPath)} }

// StreamDeltaFieldsConfig returns the environment variable name for STREAM_DELTA_FIELDS_CONFIG (brand-prefixed).
func StreamDeltaFieldsConfig() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxStreamDeltaFieldsConfig)} }

// StreamStorageEnabled returns the environment variable name for STREAM_STORAGE_ENABLED (brand-prefixed).
func StreamStorageEnabled() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxStreamStorageEnabled)} }

// TaskArtifacts returns the environment variable name for TASK_ARTIFACTS (brand-prefixed).
// Comma-separated list of target artifact/package paths for agent task validation.
func TaskArtifacts() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxTaskArtifacts)} }

// TableMaxRows returns the environment variable name for TABLE_MAX_ROWS (brand-prefixed).
func TableMaxRows() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxTableMaxRows)} }

// TestCLIBinary returns the environment variable name for TEST_CLI_BINARY (brand-prefixed).
func TestCLIBinary() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxTestCLIBinary)} }

// TestDataDir returns the environment variable name for TEST_DATA_DIR (brand-prefixed).
func TestDataDir() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxTestDataDir)} }

// TestMetricsRecording returns the environment variable name for TEST_METRICS_RECORDING (brand-prefixed).
func TestMetricsRecording() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxTestMetricsRecording)} }

// TestMode returns the environment variable name for TEST_MODE (brand-prefixed).
func TestMode() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxTestMode)} }

// TestRoot returns the environment variable name for TEST_ROOT (brand-prefixed).
//
// This names a *location*: the project root to resolve against, not a permission flag.
func TestRoot() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxTestRoot)} }

// AllowForegroundGoTest returns the environment variable name for ALLOW_FOREGROUND_GO_TEST
// (brand-prefixed). It grants exactly one thing: permission to run `go test` in the foreground
// without the agent guard panicking. It changes no paths and no behavior, which is the point —
// it is safe to export into a long-lived shell.
func AllowForegroundGoTest() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAllowForegroundGoTest)} }

// SharedTestBin returns the environment variable name for SHARED_TEST_BIN (brand-prefixed).
func SharedTestBin() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxSharedTestBin)} }

// TestVerbose returns the environment variable name for TEST_VERBOSE (brand-prefixed).
func TestVerbose() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxTestVerbose)} }

// UpdateHelpGolden returns the environment variable name for UPDATE_HELP_GOLDEN (brand-prefixed).
func UpdateHelpGolden() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxUpdateHelpGolden)} }

// StorageMode returns the environment variable name for STORAGE_MODE (brand-prefixed).
// Values: "file" (default SSOT), "file+projection" (target), "hybrid_legacy" (transitional dual-write), "graph_only".
func StorageMode() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxStorageMode)} }

// StorageModeHybridLegacy returns the environment variable name for STORAGE_MODE_HYBRID_LEGACY (brand-prefixed).
// Set to "1", "true", or "yes" to opt into legacy dual-write behavior when graph is enabled.
func StorageModeHybridLegacy() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxStorageModeHybridLegacy)} }

// ProjectionFailClosed returns the env name for PROJECTION_FAIL_CLOSED (brand-prefixed).
// When "1"/"true"/"yes", FileFirstProjectionStorage returns projection errors after file SSOT succeeds.
func ProjectionFailClosed() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxProjectionFailClosed)} }

// MCPEnvKeyPrefix returns the prefix for MCP-scoped env vars (e.g. ZQK_MCP_).
func MCPEnvKeyPrefix() string { return brand.EnvVar("MCP") + "_" }

// Unprefixed / cross-tool env names (not brand-prefixed). Use where the literal name is part of an external contract.

const (
	_rawAutofixBatchChunkSize    = "AUTOFIX_BATCH_CHUNK_SIZE"
	_rawAutofixGlossaryMaxCreate = "AUTOFIX_GLOSSARY_MAX_CREATE"
	_rawEnvHome                  = "HOME"
	_rawEnvPath                  = "PATH"
	_rawEnvUser                  = "USER"
	_rawEnvUsername              = "USERNAME"
)

// AutofixBatchChunkSize returns the env var for autofix transactional chunk size (unprefixed; scheduler/automation contract).
func AutofixBatchChunkSize() EnvVar { return EnvVar{Key: _rawAutofixBatchChunkSize} }

// AutofixGlossaryMaxCreate returns the env var capping glossary term creation after autofix (unprefixed).
func AutofixGlossaryMaxCreate() EnvVar { return EnvVar{Key: _rawAutofixGlossaryMaxCreate} }

// POSIXHome returns the conventional HOME environment variable name.
func POSIXHome() EnvVar { return EnvVar{Key: _rawEnvHome} }

// POSIXPath returns the conventional PATH environment variable name.
func POSIXPath() EnvVar { return EnvVar{Key: _rawEnvPath} }

// POSIXUser returns the conventional USER environment variable name (Unix).
func POSIXUser() EnvVar { return EnvVar{Key: _rawEnvUser} }

// POSIXUsername returns the conventional USERNAME environment variable name (Windows).
func POSIXUsername() EnvVar { return EnvVar{Key: _rawEnvUsername} }

// ZqkGraphEnabledLegacy returns the legacy ZQK_GRAPH_ENABLED toggle (tests / older tooling).
func ZqkGraphEnabledLegacy() EnvVar { return EnvVar{Key: DefaultBrandKey("GRAPH_ENABLED")} }

const _sfxSchedulerDaemonMode = "SCHEDULER_DAEMON_MODE"

func SchedulerDaemonMode() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxSchedulerDaemonMode)} }

const _sfxDemoMode = "DEMO_MODE"
const _sfxEnableAmbientWatcher = "ENABLE_AMBIENT_WATCHER"
const _sfxFalAPIKey = "FAL_API_KEY" //nolint:gosec
const _sfxMubertCompanyID = "MUBERT_COMPANY_ID"
const _sfxMubertLicToken = "MUBERT_LIC_TOKEN" //nolint:gosec
const _sfxWatchdogTimeout = "WATCHDOG_TIMEOUT"

func WatchdogTimeout() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxWatchdogTimeout)} }

const _sfxZqkShimBypassTraceability = "SHIM_BYPASS_TRACEABILITY" //nolint:gosec

// ZqkShimBypassTraceability returns the canonical environment variable name for bypassing shim commit traceability.
func ZqkShimBypassTraceability() EnvVar {
	return EnvVar{Key: brand.EnvVar(_sfxZqkShimBypassTraceability)}
}

// ZqkShimBypassPolCode009 is a deprecated alias for ZqkShimBypassTraceability (retained for backward compatibility).
const _sfxZqkShimBypassPolCode009 = "ZQK_SHIM_BYPASS_POLCODE009" //nolint:gosec

func ZqkShimBypassPolCode009() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxZqkShimBypassPolCode009)} }

const _sfxBreakGlassReason = "BREAK_GLASS_REASON"

// BreakGlassReason returns the environment variable name for BREAK_GLASS_REASON (brand-prefixed, e.g. ZQK_BREAK_GLASS_REASON).
func BreakGlassReason() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxBreakGlassReason)} }

// DemoMode returns the environment variable name for DEMO_MODE (brand-prefixed).
func DemoMode() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxDemoMode)} }

// EnableAmbientWatcher returns the environment variable name for ENABLE_AMBIENT_WATCHER (brand-prefixed).
func EnableAmbientWatcher() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxEnableAmbientWatcher)} }

const _sfxAmbientIgnoreDirs = "AMBIENT_IGNORE_DIRS"
const _sfxAmbientIgnorePaths = "AMBIENT_IGNORE_PATHS"

// AmbientIgnoreDirs returns the environment variable name for AMBIENT_IGNORE_DIRS (brand-prefixed, e.g. ZQK_AMBIENT_IGNORE_DIRS).
func AmbientIgnoreDirs() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAmbientIgnoreDirs)} }

// AmbientIgnorePaths returns the environment variable name for AMBIENT_IGNORE_PATHS (brand-prefixed, e.g. ZQK_AMBIENT_IGNORE_PATHS).
func AmbientIgnorePaths() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxAmbientIgnorePaths)} }

// FalAPIKey returns the environment variable name for FAL_API_KEY (brand-prefixed).
func FalAPIKey() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxFalAPIKey)} }

// MubertCompanyID returns the environment variable name for MUBERT_COMPANY_ID (brand-prefixed).
func MubertCompanyID() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxMubertCompanyID)} }

// MubertLicToken returns the environment variable name for MUBERT_LIC_TOKEN (brand-prefixed).
func MubertLicToken() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxMubertLicToken)} }

const _sfxTestBypassAuth = "TEST_BYPASS_AUTH" //nolint:gosec
const _sfxTestMockSuccess = "TEST_MOCK_SUCCESS"
const _sfxTestMockFailure = "TEST_MOCK_FAILURE"
const _sfxFileFallback = "FILE_FALLBACK"

// TestBypassAuth returns the environment variable name for TEST_BYPASS_AUTH (brand-prefixed).
func TestBypassAuth() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxTestBypassAuth)} }

// TestMockSuccess returns the environment variable name for TEST_MOCK_SUCCESS (brand-prefixed).
func TestMockSuccess() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxTestMockSuccess)} }

// TestMockFailure returns the environment variable name for TEST_MOCK_FAILURE (brand-prefixed).
func TestMockFailure() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxTestMockFailure)} }

// FileFallback returns the environment variable name for FILE_FALLBACK (brand-prefixed).
func FileFallback() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxFileFallback)} }

const _sfxLLMProvider = "LLM_PROVIDER"
const _sfxGeminiAPIKey = "GEMINI_API_KEY" //nolint:gosec

// LLMProvider returns the environment variable name for LLM_PROVIDER (brand-prefixed).
func LLMProvider() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxLLMProvider)} }

// GeminiAPIKey returns the environment variable name for GEMINI_API_KEY (brand-prefixed).
func GeminiAPIKey() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxGeminiAPIKey)} }

const _sfxTestSkipValidation = "TEST_SKIP_VALIDATION"
const _sfxTestAllowCASFallthrough = "TEST_ALLOW_CAS_FALLTHROUGH"
const _sfxPrivilegedWriterSocket = "PRIVILEGED_WRITER_SOCKET"

func TestSkipValidation() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxTestSkipValidation)} }

// TestAllowCASFallthrough returns the env name for TEST_ALLOW_CAS_FALLTHROUGH (brand-prefixed).
// When set to "1", CAS create/update/delete may write locally if PrivilegedWriter is unreachable.
func TestAllowCASFallthrough() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxTestAllowCASFallthrough)} }

// PrivilegedWriterSocket returns the env name for PRIVILEGED_WRITER_SOCKET (brand-prefixed).
// When set, overrides the default UNIX socket path for the PrivilegedWriter helper.
func PrivilegedWriterSocket() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxPrivilegedWriterSocket)} }

const _sfxIsParentZqk = "IS_PARENT_ZQK"

// IsParentZqk returns the environment variable name for IS_PARENT_ZQK (brand-prefixed).
func IsParentZqk() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxIsParentZqk)} }

const _sfxIsDaemon = "IS_DAEMON"

// IsDaemon returns the env name for IS_DAEMON (brand-prefixed). Set to "1" by the privileged
// writer daemon so in-process storage writes locally (no self-dial), skips write-behind WAL
// replay, and takes the async hash-registry save path.
func IsDaemon() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxIsDaemon)} }

const _sfxPrototypeAccountRefs = "PROTOTYPE_ACCOUNT_REFS"

// PrototypeAccountRefs returns the env name for PROTOTYPE_ACCOUNT_REFS (brand-prefixed).
// Comma-separated account refs that must be rejected as prototype/test accounts in production
// (see pkg/authcred.RegisterPrototypeAccountRefs).
func PrototypeAccountRefs() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxPrototypeAccountRefs)} }

const _sfxTDDDiffTarget = "TDD_DIFF_TARGET"

// TDDDiffTarget returns the environment variable name for TDD_DIFF_TARGET (brand-prefixed).
func TDDDiffTarget() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxTDDDiffTarget)} }

const _sfxEnableStudioPackTools = "ENABLE_STUDIO_PACK_TOOLS"

// EnableStudioPackTools returns the environment variable name for ENABLE_STUDIO_PACK_TOOLS (brand-prefixed).
func EnableStudioPackTools() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxEnableStudioPackTools)} }

const _sfxStudioDogfood = "STUDIO_DOGFOOD"

// StudioDogfood returns the environment variable name for STUDIO_DOGFOOD (brand-prefixed).
func StudioDogfood() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxStudioDogfood)} }

const _sfxTestBypassGitevidence = "TEST_BYPASS_GITEVIDENCE"

// TestBypassGitevidence returns the environment variable name for TEST_BYPASS_GITEVIDENCE (brand-prefixed).
func TestBypassGitevidence() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxTestBypassGitevidence)} }

const _sfxSkipStateCommit = "SKIP_STATE_COMMIT"

// SkipStateCommit returns the environment variable name for SKIP_STATE_COMMIT (brand-prefixed).
func SkipStateCommit() EnvVar { return EnvVar{Key: brand.EnvVar(_sfxSkipStateCommit)} }

// Standard OS environment variables
func OSHome() EnvVar    { return EnvVar{Key: "HOME"} }
func OSPath() EnvVar    { return EnvVar{Key: "PATH"} }
func OSEditor() EnvVar  { return EnvVar{Key: "EDITOR"} }
func OSShell() EnvVar   { return EnvVar{Key: "SHELL"} }
func OSOS() EnvVar      { return EnvVar{Key: "OS"} }
func OSAppData() EnvVar { return EnvVar{Key: "APPDATA"} }

// Toolchain environment variables
func GoDebug() EnvVar       { return EnvVar{Key: "GODEBUG"} }
func EnvCI() EnvVar         { return EnvVar{Key: "CI"} }
func GithubActions() EnvVar { return EnvVar{Key: "GITHUB_ACTIONS"} }
func GitlabCI() EnvVar      { return EnvVar{Key: "GITLAB_CI"} }
func MakeFlags() EnvVar     { return EnvVar{Key: "MAKEFLAGS"} }
func MakeLevel() EnvVar     { return EnvVar{Key: "MAKELEVEL"} }

// Third-party API keys and URLs
func FalKey() EnvVar        { return EnvVar{Key: "FAL_KEY"} }
func RawFalAPIKey() EnvVar  { return EnvVar{Key: "FAL_API_KEY"} }
func FalBaseURL() EnvVar    { return EnvVar{Key: "FAL_BASE_URL"} }
func FalModelPath() EnvVar  { return EnvVar{Key: "FAL_MODEL_PATH"} }
func OpenAIAPIKey() EnvVar  { return EnvVar{Key: "OPENAI_API_KEY"} }
func OpenAIBaseURL() EnvVar { return EnvVar{Key: "OPENAI_BASE_URL"} }
func MubertBaseURL() EnvVar { return EnvVar{Key: "MUBERT_BASE_URL"} }

// Hardware and Network environment variables
func NvidiaVisibleDevices() EnvVar { return EnvVar{Key: "NVIDIA_VISIBLE_DEVICES"} }
func SSHConnection() EnvVar        { return EnvVar{Key: "SSH_CONNECTION"} }
func SSHClient() EnvVar            { return EnvVar{Key: "SSH_CLIENT"} }

// Internal / Domain-specific overrides without ZQK prefix
func AGTranscriptPath() EnvVar           { return EnvVar{Key: "AG_TRANSCRIPT_PATH"} }
func RawMockGraph() EnvVar               { return EnvVar{Key: "MOCK_GRAPH"} }
func RawGraphEnabled() EnvVar            { return EnvVar{Key: "GRAPH_ENABLED"} }
func MCPRunDeadlockReproduction() EnvVar { return EnvVar{Key: "MCP_RUN_DEADLOCK_REPRODUCTION"} }

// Aliases for prefix-less internal vars that need a specific ZQK-equivalent function
func ZQKCLITestUpdateHelpGolden() EnvVar { return EnvVar{Key: "ZQKCLI_TEST_UPDATE_HELP_GOLDEN"} }
func ZQKAllowForegroundGoTest() EnvVar   { return EnvVar{Key: "ZQK_ALLOW_FOREGROUND_GO_TEST"} }
func ZqkEnv() EnvVar                     { return Env() }
func ZQKProjectRoot() EnvVar             { return EnvVar{Key: DefaultBrandKey("PROJECT_ROOT")} }
func ZQKTestRoot() EnvVar                { return EnvVar{Key: DefaultBrandKey("TEST_ROOT")} }
func WorkerLane() EnvVar                 { return EnvVar{Key: brand.EnvVar("WORKER_LANE")} }
func SeatWorkerLane() EnvVar             { return EnvVar{Key: brand.EnvVar("SEAT_WORKER_LANE")} }
func WorkerLanes() EnvVar                { return EnvVar{Key: brand.EnvVar("WORKER_LANES")} }
