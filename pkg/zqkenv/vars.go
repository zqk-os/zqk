package zqkenv

import (
	"flag"
	"os"
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/pkg/brand"
)

// IsCommunityEdition is a global flag set by the community binary.
var IsCommunityEdition bool = false

const _sfxAccountID = "ACCOUNT_ID"

const _sfxStressRealRoot = "STRESS_REAL_ROOT"
const _sfxAgentPromptDeliveryHTTPBearer = "AGENT_PROMPT_DELIVERY_HTTP_BEARER"
const _sfxAgentPromptDeliveryHTTPURL = "AGENT_PROMPT_DELIVERY_HTTP_URL"
const _sfxAgentPromptKeystrokeLog = "AGENT_PROMPT_KEYSTROKE_LOG"
const _sfxAgentRulesDir = "AGENT_RULES_DIR"
const _sfxAgentWorktreeRoot = "AGENT_WORKTREE_ROOT"
const _sfxAgentID = "AGENT_ID"
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

const _sfxCacheDiagnosticEnabled = "CACHE_DIAGNOSTIC_ENABLED"
const _sfxCacheDiagnosticObjects = "CACHE_DIAGNOSTIC_OBJECTS"
const _sfxCacheDiagnosticPrefixes = "CACHE_DIAGNOSTIC_PREFIXES"
const _sfxIDEPastePrefixSteps = "CURSOR_PASTE_PREFIX_STEPS"
const _sfxIDEPasteApp = "IDE_PASTE_APP"
const _sfxCVSPerTestLedgerMaxFailures = "CVS_LEDGER_MAX_FAILURES"
const _sfxCRUDBaselineCount = "CRUD_BASELINE_COUNT"
const _sfxDisableCriteriaAutoValidate = "DISABLE_CRITERIA_AUTO_VALIDATE"
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
const _sfxEnableHashRegistryCoordinatorTests = "ENABLE_HASH_REGISTRY_COORDINATOR_TESTS"
const _sfxEnableMigrateLegacyToStreamIntegrationTest = "ENABLE_MIGRATE_LEGACY_TO_STREAM_INTEGRATION_TEST"
const _sfxEnableIOQueueShutdownTests = "ENABLE_IOQUEUE_SHUTDOWN_TESTS"
const _sfxEnablePublicCandidateTest = "ENABLE_PUBLIC_CANDIDATE_TEST"
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
const _sfxLLMAPIKey = "LLM_API_KEY"
const _sfxLLMBaseURL = "LLM_BASE_URL"
const _sfxLLMChatModel = "LLM_CHAT_MODEL"
const _sfxLLMContextWindowSize = "LLM_CONTEXT_WINDOW_SIZE"
const _sfxSwarmMaxSteps = "SWARM_MAX_STEPS"
const _sfxLLMEmbedModel = "LLM_EMBED_MODEL"
const _sfxLLMTrace = "LLM_TRACE"
const _sfxLLMTraceDir = "LLM_TRACE_DIR"
const _sfxLLMTemperature = "LLM_TEMPERATURE"
const _sfxLLMTopP = "LLM_TOP_P"
const _sfxQwenBaseURL = "QWEN_BASE_URL"
const _sfxQwenAPIKey = "QWEN_API_KEY"
const _sfxMetricsChunkRetentionDays = "METRICS_CHUNK_RETENTION_DAYS"
const _sfxMCPAccountID = "MCP_ACCOUNT_ID"
const _sfxMCPConfigPath = "MCP_CONFIG_PATH"
const _sfxMCPKeystoreKeyID = "MCP_KEYSTORE_KEY_ID"
const _sfxMCPExternalSecretKey = "MCP_EXTERNAL_SECRET_KEY"
const _sfxMCPPermissions = "MCP_PERMISSIONS"
const _sfxMCPRoles = "MCP_ROLES"
const _sfxMCPTrace = "MCP_TRACE"
const _sfxMCPTraceFile = "MCP_TRACE_FILE"
const _sfxParentPID = "PARENT_PID"
const _sfxPersona = "PERSONA"
const _sfxPopulateScenarioTest = "POPULATE_SCENARIO_TEST"
const _sfxPprof = "PPROF"
const _sfxPprofPort = "PPROF_PORT"
const _sfxProjectRoot = "PROJECT_ROOT"
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
const _sfxTestCLIBinary = "TEST_CLI_BINARY"
const _sfxTestDataDir = "TEST_DATA_DIR"
const _sfxTestMetricsRecording = "TEST_METRICS_RECORDING"
const _sfxTestMode = "TEST_MODE"
const _sfxTestRoot = "TEST_ROOT"
const _sfxTestVerbose = "TEST_VERBOSE"
const _sfxUpdateHelpGolden = "UPDATE_HELP_GOLDEN"

// AccountID returns the environment variable name for ACCOUNT_ID (brand-prefixed).
func AccountID() string { return brand.EnvVar(_sfxAccountID) }

// StressRealRoot returns the env var for STRESS_REAL_ROOT (brand-prefixed).
// When set to "1", optional stress tests may use the live checkout root.
func StressRealRoot() string { return brand.EnvVar(_sfxStressRealRoot) }

// APIKey returns the environment variable name for API_KEY (brand-prefixed).
func APIKey() string { return brand.EnvVar(_sfxAPIKey) }

// AgentPromptKeystrokeLog returns the environment variable name for AGENT_PROMPT_KEYSTROKE_LOG (brand-prefixed).
// Passed to osascript for optional AppleScript consumers (path to agent_prompt_ide_keystrokes.log).
func AgentPromptKeystrokeLog() string { return brand.EnvVar(_sfxAgentPromptKeystrokeLog) }

// AgentPromptDeliveryHTTPURL returns the environment variable name for AGENT_PROMPT_DELIVERY_HTTP_URL (brand-prefixed).
// Optional webhook URL for agent-prompt markdown POST (see pkg/agentdelivery.HTTPDeliverer).
func AgentPromptDeliveryHTTPURL() string { return brand.EnvVar(_sfxAgentPromptDeliveryHTTPURL) }

// AgentPromptDeliveryHTTPBearer returns the environment variable name for AGENT_PROMPT_DELIVERY_HTTP_BEARER (brand-prefixed).
// Optional Bearer token for Authorization when posting agent-prompt markdown to HTTP.
func AgentPromptDeliveryHTTPBearer() string { return brand.EnvVar(_sfxAgentPromptDeliveryHTTPBearer) }

// AgentRulesDir returns the environment variable name for AGENT_RULES_DIR (brand-prefixed).
// Optional path relative to project root for tracked *.mdc rule files (default .ide/rules when unset).
// Used by zqk system validate-agent-rules; prefer no env when using the repo default layout.
func AgentRulesDir() string { return brand.EnvVar(_sfxAgentRulesDir) }

// AgentWorktreeRoot returns the env var name for AGENT_WORKTREE_ROOT (brand-prefixed).
// Optional absolute base for isolated agent git worktrees. Must not be under the
// studio project root. Default when unset: $TMPDIR/zqk-worktrees/<repo-key>/.
// Kernel: POL-AGENT-WORKTREE-ISOLATION-001.
func AgentWorktreeRoot() string { return brand.EnvVar(_sfxAgentWorktreeRoot) }

// AgentID returns the environment variable name for AGENT_ID (brand-prefixed).
func AgentID() string { return brand.EnvVar(_sfxAgentID) }

// AgentSyncMaxLoops returns the env var for AGENT_SYNC_MAX_LOOPS (brand-prefixed).
// Sync-loop outer poll guard; default 100. TRACK: REDACTED
func AgentSyncMaxLoops() string { return brand.EnvVar(_sfxAgentSyncMaxLoops) }

// AgentMaxVerificationAttempts returns AGENT_MAX_VERIFICATION_ATTEMPTS (brand-prefixed).
// Per-step verification retry cap; default 3. TRACK: REDACTED
func AgentMaxVerificationAttempts() string { return brand.EnvVar(_sfxAgentMaxVerificationAttempts) }

// AgentSyncMaxStagnantTicks returns AGENT_SYNC_MAX_STAGNANT_TICKS (brand-prefixed).
// Abort when task progress fingerprint is unchanged this many ticks; default 10.
// TRACK: REDACTED
func AgentSyncMaxStagnantTicks() string { return brand.EnvVar(_sfxAgentSyncMaxStagnantTicks) }

// AgentPubKey returns the environment variable name for AGENT_PUB_KEY (brand-prefixed).
func AgentPubKey() string { return brand.EnvVar(_sfxAgentPubKey) }

// AgentPrivateKey returns the environment variable name for AGENT_PRIVATE_KEY (brand-prefixed).
func AgentPrivateKey() string { return brand.EnvVar(_sfxAgentPrivateKey) }

// AggregationMetricCreationTimeout returns the environment variable name for AGGREGATION_METRIC_CREATION_TIMEOUT (brand-prefixed).
func AggregationMetricCreationTimeout() string {
	return brand.EnvVar(_sfxAggregationMetricCreationTimeout)
}

// Bin returns the environment variable name for BIN (brand-prefixed).
func Bin() string { return brand.EnvVar(_sfxBin) }

// BulkBenchSize returns the environment variable name for BULK_BENCH_SIZE (brand-prefixed).
func BulkBenchSize() string { return brand.EnvVar(_sfxBulkBenchSize) }

// CacheDiagnosticEnabled returns the environment variable name for CACHE_DIAGNOSTIC_ENABLED (brand-prefixed).
func CacheDiagnosticEnabled() string { return brand.EnvVar(_sfxCacheDiagnosticEnabled) }

// CacheDiagnosticObjects returns the environment variable name for CACHE_DIAGNOSTIC_OBJECTS (brand-prefixed).
func CacheDiagnosticObjects() string { return brand.EnvVar(_sfxCacheDiagnosticObjects) }

// CacheDiagnosticPrefixes returns the environment variable name for CACHE_DIAGNOSTIC_PREFIXES (brand-prefixed).
func CacheDiagnosticPrefixes() string { return brand.EnvVar(_sfxCacheDiagnosticPrefixes) }

// IDEPastePrefixSteps returns the environment variable name for CURSOR_PASTE_PREFIX_STEPS (brand-prefixed).
// Comma-separated keystroke tokens before ⌘V in IDE paste automation (escape, cmd_l, option_cmd_e).
// See cmd/zqk/scheduler/ide_paste_automation.go and scripts/ide/README.md.
func IDEPastePrefixSteps() string { return brand.EnvVar(_sfxIDEPastePrefixSteps) }

// IDEPasteApp returns the environment variable name for IDE_PASTE_APP (brand-prefixed).
// macOS application / System Events process name targeted by paste AppleScript (default Cursor).
func IDEPasteApp() string { return brand.EnvVar(_sfxIDEPasteApp) }

// CVSPerTestLedgerMaxFailures returns the env var name for CVS_LEDGER_MAX_FAILURES (brand-prefixed).
// Caps how many parsed failed tests are recorded under convergence_session.after_state_snapshot.per_test_failure_ledger_v1.
// Omit or invalid values use the scheduler default; set to 0 for no limit (skip threshold).
func CVSPerTestLedgerMaxFailures() string { return brand.EnvVar(_sfxCVSPerTestLedgerMaxFailures) }

// CRUDBaselineCount returns the environment variable name for CRUD_BASELINE_COUNT (brand-prefixed).
func CRUDBaselineCount() string { return brand.EnvVar(_sfxCRUDBaselineCount) }

// DisableCriteriaAutoValidate returns the environment variable name for DISABLE_CRITERIA_AUTO_VALIDATE (brand-prefixed).
// When set to 1/true, the scheduler does not apply criteria status=validated from green SCH-run-* test bundles
// (criteria_verification_satisfied). Unset: allow auto-apply (opt-out).
func DisableCriteriaAutoValidate() string { return brand.EnvVar(_sfxDisableCriteriaAutoValidate) }

// HostloadDisable returns the env name for HOSTLOAD_DISABLE (brand-prefixed).
// When set to 1, scheduler and validation fan-out ignore live host CPU pressure
// (always treat the host as having headroom). TRACK: TDE-CEF-HOST-CPU-BACKPRESSURE-001
func HostloadDisable() string { return brand.EnvVar(_sfxHostloadDisable) }

// HostCPUBackpressure returns the environment variable name for HOST_CPU_BACKPRESSURE (brand-prefixed).
func HostCPUBackpressure() string { return brand.EnvVar(_sfxHostCPUBackpressure) }

// CRUDHeapProfile returns the environment variable name for CRUD_HEAP_PROFILE (brand-prefixed).
func CRUDHeapProfile() string { return brand.EnvVar(_sfxCRUDHeapProfile) }

// CRUDProfile returns the environment variable name for CRUD_PROFILE (brand-prefixed).
func CRUDProfile() string { return brand.EnvVar(_sfxCRUDProfile) }

// DebugValidationObjectIDs returns the environment variable name for DEBUG_VALIDATION_OBJECT_IDS (brand-prefixed).
func DebugValidationObjectIDs() string { return brand.EnvVar(_sfxDebugValidationObjectIDs) }

// DiagnosticsDir returns the environment variable name for DIAGNOSTICS_DIR (brand-prefixed).
func DiagnosticsDir() string { return brand.EnvVar(_sfxDiagnosticsDir) }

// DiagnosticsThreadStackSkipMinGoroutines returns the environment variable name for DIAGNOSTICS_THREAD_STACK_SKIP_MIN_GOROUTINES (brand-prefixed).
// CaptureDiagnostics uses this threshold to skip redundant runtime.Stack output in *_threads.txt when NumGoroutine() is at or above this value.
// Positive integer: skip when goroutine count >= value. Zero or negative: do not skip on threshold (always attempt runtime.Stack).
// Unset: use the package default (see pkg/diagnostics threadDumpSkipRuntimeStackMinGoroutines).
func DiagnosticsThreadStackSkipMinGoroutines() string {
	return brand.EnvVar(_sfxDiagnosticsThreadStackSkipMin)
}

// EnableBootstrapCRUDTests returns the environment variable name for ENABLE_BOOTSTRAP_CRUD_TESTS (brand-prefixed).
func EnableBootstrapCRUDTests() string { return brand.EnvVar(_sfxEnableBootstrapCRUDTests) }

// EnableSpecCellIntegrationTests returns the environment variable name for ENABLE_SPEC_CELL_INTEGRATION_TESTS (brand-prefixed).
// When set to 1/true/yes, runs integration tests for greenfield init, bootstrap specs, spec index, and update-specs (data cell / alpha launch gate suite).
func EnableSpecCellIntegrationTests() string { return brand.EnvVar(_sfxEnableSpecCellIntegrationTests) }

// EnableSpecCellREQ019Validate returns the environment variable name for ENABLE_SPEC_CELL_REQ019_VALIDATE (brand-prefixed).
// When set together with [EnableSpecCellIntegrationTests], field define runs update-specs with --validate (REQ-019 / LoadSpecAndValidate).
// Default off: bundled bootstrap specs still carry inherited-field checklist debt, so full-spec validation fails until that debt is cleared.
func EnableSpecCellREQ019Validate() string { return brand.EnvVar(_sfxEnableSpecCellREQ019Validate) }

// EnableCASMigrationScenarioTest returns the environment variable name for ENABLE_CAS_MIGRATION_SCENARIO_TEST (brand-prefixed).
func EnableCASMigrationScenarioTest() string { return brand.EnvVar(_sfxEnableCASMigrationScenarioTest) }

// EnableCLIScenarioTests returns the environment variable name for ENABLE_CLI_SCENARIO_TESTS (brand-prefixed).
func EnableCLIScenarioTests() string { return brand.EnvVar(_sfxEnableCLIScenarioTests) }

// EnableHashRegistryCoordinatorTests returns the environment variable name for ENABLE_HASH_REGISTRY_COORDINATOR_TESTS (brand-prefixed).
func EnableHashRegistryCoordinatorTests() string {
	return brand.EnvVar(_sfxEnableHashRegistryCoordinatorTests)
}

// EnableIOQueueShutdownTests returns the environment variable name for ENABLE_IOQUEUE_SHUTDOWN_TESTS (brand-prefixed).
func EnableIOQueueShutdownTests() string { return brand.EnvVar(_sfxEnableIOQueueShutdownTests) }

// EnablePublicCandidateTest returns the environment variable name for ENABLE_PUBLIC_CANDIDATE_TEST (brand-prefixed).
// When set to 1, runs the open-core public-candidate sync/police integration test.
func EnablePublicCandidateTest() string { return brand.EnvVar(_sfxEnablePublicCandidateTest) }

// EnableMigrateLegacyToStreamIntegrationTest returns the environment variable name for ENABLE_MIGRATE_LEGACY_TO_STREAM_INTEGRATION_TEST (brand-prefixed).
// When set to 1/true/yes, runs cmd/zqk/system.TestMigrateLegacyToStream_subprocessIntegration (greenfield init, legacy audit_event YAML, subprocess migrate-legacy-to-stream).
func EnableMigrateLegacyToStreamIntegrationTest() string {
	return brand.EnvVar(_sfxEnableMigrateLegacyToStreamIntegrationTest)
}

// GraphDatabase returns the environment variable name for GRAPH_DATABASE (brand-prefixed).
func GraphDatabase() string { return brand.EnvVar(_sfxGraphDatabase) }

// GraphEnabled returns the environment variable name for GRAPH_ENABLED (brand-prefixed).
func GraphEnabled() string { return brand.EnvVar(_sfxGraphEnabled) }

// AdminGraphEnabled returns the environment variable name for ADMIN_GRAPH_ENABLED (brand-prefixed).
func AdminGraphEnabled() string { return brand.EnvVar(_sfxAdminGraphEnabled) }

// GraphHost returns the environment variable name for GRAPH_HOST (brand-prefixed).
func GraphHost() string { return brand.EnvVar(_sfxGraphHost) }

// GraphPassword returns the environment variable name for GRAPH_PASSWORD (brand-prefixed).
func GraphPassword() string { return brand.EnvVar(_sfxGraphPassword) }

// GraphPoolSize returns the environment variable name for GRAPH_POOL_SIZE (brand-prefixed).
func GraphPoolSize() string { return brand.EnvVar(_sfxGraphPoolSize) }

// GraphPort returns the environment variable name for GRAPH_PORT (brand-prefixed).
func GraphPort() string { return brand.EnvVar(_sfxGraphPort) }

// GraphUsername returns the environment variable name for GRAPH_USERNAME (brand-prefixed).
func GraphUsername() string { return brand.EnvVar(_sfxGraphUsername) }

// MockGraph returns the environment variable name for MOCK_GRAPH (brand-prefixed).
func MockGraph() string { return brand.EnvVar(_sfxMockGraph) }

// AdminMockGraph returns the environment variable name for ADMIN_MOCK_GRAPH (brand-prefixed).
func AdminMockGraph() string { return brand.EnvVar(_sfxAdminMockGraph) }

// LegacyMockGraph returns the unprefixed MOCK_GRAPH key that graph validation still honors alongside
// the brand-prefixed one. Exposed so callers do not restate the bare literal.
func LegacyMockGraph() string { return _sfxMockGraph }

// FFMPEGWorkerEndpoint returns the environment variable name for FFMPEG_WORKER_ENDPOINT (brand-prefixed).
func FFMPEGWorkerEndpoint() string { return brand.EnvVar(_sfxFFMPEGWorkerEndpoint) }

// HighVolumeCacheBuildWorkers returns the environment variable name for HIGH_VOLUME_CACHE_BUILD_WORKERS (brand-prefixed).
func HighVolumeCacheBuildWorkers() string { return brand.EnvVar(_sfxHighVolumeCacheBuildWorkers) }

// HighVolumeCacheKindParallelism returns the environment variable name for HIGH_VOLUME_CACHE_KIND_PARALLELISM (brand-prefixed).
// Bounds concurrent high-volume kinds during cache build (each kind still uses parallel index workers).
func HighVolumeCacheKindParallelism() string { return brand.EnvVar(_sfxHighVolumeCacheKindParallelism) }

// InTest returns the environment variable name for IN_TEST (brand-prefixed).
// When set to "true", indicates the process is running inside a test harness.
func InTest() string { return brand.EnvVar(_sfxInTest) }

// IsInTest returns true if the IN_TEST environment variable is set to "true",
// or if the process is detected to be running under a test runner.
func IsInTest() bool {
	if os.Getenv(brand.EnvVar(_sfxInTest)) == "true" {
		return true
	}
	// Check if running under go test
	if flag.Lookup("test.v") != nil {
		return true
	}
	// Fallback to checking executable suffix
	if len(os.Args) > 0 {
		base := strings.ToLower(filepath.Base(os.Args[0]))
		if strings.HasSuffix(base, ".test") ||
			strings.Contains(os.Args[0], "go-build") ||
			strings.Contains(os.Args[0], "___go_build") ||
			strings.Contains(base, "test") {
			return true
		}
	}
	return false
}

// JobID returns the environment variable name for JOB_ID (brand-prefixed).
func JobID() string { return brand.EnvVar(_sfxJobID) }

// SchedulerJobID returns the environment variable name for SCHEDULER_JOB_ID (brand-prefixed).
// Legacy name used by run-tests-bg.sh; prefer JobID() for new code.
func SchedulerJobID() string { return brand.EnvVar(_sfxSchedulerJobID) }

// Persona returns the environment variable name for PERSONA (brand-prefixed).
func Persona() string { return brand.EnvVar(_sfxPersona) }

// ListCountMaxConcurrent returns the environment variable name for LIST_COUNT_MAX_CONCURRENT (brand-prefixed).
func ListCountMaxConcurrent() string { return brand.EnvVar(_sfxListCountMaxConcurrent) }

// ListReadWorkers returns the environment variable name for LIST_READ_WORKERS (brand-prefixed).
func ListReadWorkers() string { return brand.EnvVar(_sfxListReadWorkers) }

// MaxOSThreads returns the environment variable name for MAX_OS_THREADS (brand-prefixed).
func MaxOSThreads() string { return brand.EnvVar(_sfxMaxOSThreads) }

// MaxObjectYAMLIO returns the environment variable name for MAX_OBJECT_YAML_IO (brand-prefixed).
func MaxObjectYAMLIO() string { return brand.EnvVar(_sfxMaxObjectYAMLIO) }

func QwenBaseURL() string { return brand.EnvVar(_sfxQwenBaseURL) }
func QwenAPIKey() string  { return brand.EnvVar(_sfxQwenAPIKey) }

// SwarmMaxSteps returns the environment variable name for SWARM_MAX_STEPS (brand-prefixed).
func SwarmMaxSteps() string { return brand.EnvVar(_sfxSwarmMaxSteps) }

// LLMAPIKey returns the environment variable name for LLM_API_KEY (brand-prefixed).
func LLMAPIKey() string { return brand.EnvVar(_sfxLLMAPIKey) }

// LLMBaseURL returns the environment variable name for LLM_BASE_URL (brand-prefixed).
func LLMBaseURL() string { return brand.EnvVar(_sfxLLMBaseURL) }

// LLMChatModel returns the environment variable name for LLM_CHAT_MODEL (brand-prefixed).
func LLMChatModel() string { return brand.EnvVar(_sfxLLMChatModel) }

// LLMContextWindowSize returns the environment variable name for LLM_CONTEXT_WINDOW_SIZE (brand-prefixed).
func LLMContextWindowSize() string { return brand.EnvVar(_sfxLLMContextWindowSize) }

// LLMEmbedModel returns the environment variable name for LLM_EMBED_MODEL (brand-prefixed).
func LLMEmbedModel() string { return brand.EnvVar(_sfxLLMEmbedModel) }

// LLMTrace returns the environment variable name for LLM_TRACE (brand-prefixed).
// Unset in a real process enables swarm prompt/response traces; "0" disables;
// "1" forces on (including under go test when a dir is also set).
func LLMTrace() string { return brand.EnvVar(_sfxLLMTrace) }

// LLMTraceDir returns the environment variable name for LLM_TRACE_DIR (brand-prefixed).
// When set, swarm writes llm-trace-<engineID>.log there. Otherwise DiagnosticsDir
// or <project>/.zqk/logs/llm.
func LLMTraceDir() string { return brand.EnvVar(_sfxLLMTraceDir) }

// LLMTemperature returns the environment variable name for LLM_TEMPERATURE (brand-prefixed).
// Optional float sent as OpenAI temperature when set. Empty omits the field.
func LLMTemperature() string { return brand.EnvVar(_sfxLLMTemperature) }

// LLMTopP returns the environment variable name for LLM_TOP_P (brand-prefixed).
// Optional float sent as OpenAI top_p when set. Empty omits the field.
func LLMTopP() string { return brand.EnvVar(_sfxLLMTopP) }

// MetricsChunkRetentionDays returns the environment variable name for METRICS_CHUNK_RETENTION_DAYS (brand-prefixed).
// When set to a positive integer, object-count-report prunes .zqk/metrics/{object_volume,stream_volume,filesystem_snapshot}/*.chunk older than this many days. Used by scheduled runs and CLI.
func MetricsChunkRetentionDays() string { return brand.EnvVar(_sfxMetricsChunkRetentionDays) }

// MCPAccountID returns the environment variable name for MCP_ACCOUNT_ID (brand-prefixed).
func MCPAccountID() string { return brand.EnvVar(_sfxMCPAccountID) }

// MCPConfigPath returns the environment variable name for MCP_CONFIG_PATH (brand-prefixed).
func MCPConfigPath() string { return brand.EnvVar(_sfxMCPConfigPath) }

// MCPKeystoreKeyID returns the environment variable name for MCP_KEYSTORE_KEY_ID (brand-prefixed).
func MCPKeystoreKeyID() string { return brand.EnvVar(_sfxMCPKeystoreKeyID) }

// MCPPermissions returns the environment variable name for MCP_PERMISSIONS (brand-prefixed).
func MCPExternalSecretKey() string { return brand.EnvVar(_sfxMCPExternalSecretKey) }

func MCPPermissions() string { return brand.EnvVar(_sfxMCPPermissions) }

// MCPRoles returns the environment variable name for MCP_ROLES (brand-prefixed).
func MCPRoles() string { return brand.EnvVar(_sfxMCPRoles) }

// MCPTrace returns the environment variable name for MCP_TRACE (brand-prefixed).
func MCPTrace() string { return brand.EnvVar(_sfxMCPTrace) }

// MCPTraceFile returns the environment variable name for MCP_TRACE_FILE (brand-prefixed).
func MCPTraceFile() string { return brand.EnvVar(_sfxMCPTraceFile) }

// ParentPID returns the environment variable name for PARENT_PID (brand-prefixed).
func ParentPID() string { return brand.EnvVar(_sfxParentPID) }

// PopulateScenarioTest returns the environment variable name for POPULATE_SCENARIO_TEST (brand-prefixed).
func PopulateScenarioTest() string { return brand.EnvVar(_sfxPopulateScenarioTest) }

// Pprof returns the environment variable name for PPROF (brand-prefixed).
func Pprof() string { return brand.EnvVar(_sfxPprof) }

// PprofPort returns the environment variable name for PPROF_PORT (brand-prefixed).
func PprofPort() string { return brand.EnvVar(_sfxPprofPort) }

// ProjectRoot returns the environment variable name for PROJECT_ROOT (brand-prefixed).
func ProjectRoot() string { return brand.EnvVar(_sfxProjectRoot) }

// RetentionToleranceConfig returns the environment variable name for RETENTION_TOLERANCE_CONFIG (brand-prefixed).
func RetentionToleranceConfig() string { return brand.EnvVar(_sfxRetentionToleranceConfig) }

// RollbackCaptureDisabled returns the environment variable name for ROLLBACK_CAPTURE_DISABLED (brand-prefixed).
func RollbackCaptureDisabled() string { return brand.EnvVar(_sfxRollbackCaptureDisabled) }

// RollbackRetainCount returns the environment variable name for ROLLBACK_RETAIN_COUNT (brand-prefixed).
func RollbackRetainCount() string { return brand.EnvVar(_sfxRollbackRetainCount) }

// RollbackRetainDuration returns the environment variable name for ROLLBACK_RETAIN_DURATION (brand-prefixed).
func RollbackRetainDuration() string { return brand.EnvVar(_sfxRollbackRetainDuration) }

// Root returns the environment variable name for ROOT (brand-prefixed).
func Root() string { return brand.EnvVar(_sfxRoot) }

// RunPerfTests returns the environment variable name for RUN_PERF_TESTS (brand-prefixed).
func RunPerfTests() string { return brand.EnvVar(_sfxRunPerfTests) }

// SchedulerAdmissionTimeout returns the environment variable name for SCHEDULER_ADMISSION_TIMEOUT (brand-prefixed).
// Duration string for [time.ParseDuration] (e.g. "60s", "2m"); how long a callback-bearing CLI one-shot
// may wait to start before callback_on_error fires and the job is disabled.
func SchedulerAdmissionTimeout() string {
	return brand.EnvVar(_sfxSchedulerAdmissionTimeout)
}

// SchedulerDaemonBin returns the environment variable name for SCHEDULER_DAEMON_BIN (brand-prefixed).
func SchedulerDaemonBin() string { return brand.EnvVar(_sfxSchedulerDaemonBin) }

// SchedulerDefaultPackageConcurrency returns the environment variable name for SCHEDULER_DEFAULT_PACKAGE_CONCURRENCY (brand-prefixed).
func SchedulerDefaultPackageConcurrency() string {
	return brand.EnvVar(_sfxSchedulerDefaultPackageConcurrency)
}

// SchedulerDispatchResourceWaitMax returns the environment variable name for SCHEDULER_DISPATCH_RESOURCE_WAIT_MAX (brand-prefixed).
func SchedulerDispatchResourceWaitMax() string {
	return brand.EnvVar(_sfxSchedulerDispatchResourceWaitMax)
}

// SchedulerGoroutineCap returns the environment variable name for SCHEDULER_GOROUTINE_CAP (brand-prefixed).
func SchedulerGoroutineCap() string { return brand.EnvVar(_sfxSchedulerGoroutineCap) }

// SchedulerImmediateLoadBatchPause returns the environment variable name for SCHEDULER_IMMEDIATE_LOAD_BATCH_PAUSE (brand-prefixed).
// Optional duration between batches when immediate-load chunking is enabled (see SchedulerImmediateLoadBatchSize).
func SchedulerImmediateLoadBatchPause() string {
	return brand.EnvVar(_sfxSchedulerImmediateLoadBatchPause)
}

// SchedulerImmediateLoadBatchSize returns the environment variable name for SCHEDULER_IMMEDIATE_LOAD_BATCH_SIZE (brand-prefixed).
// When > 0, initial/reload scheduling of immediate jobs runs in chunks of this size with optional pause between chunks.
func SchedulerImmediateLoadBatchSize() string {
	return brand.EnvVar(_sfxSchedulerImmediateLoadBatchSize)
}

// SchedulerLogsConfig returns the environment variable name for SCHEDULER_LOGS_CONFIG (brand-prefixed).
func SchedulerLogsConfig() string { return brand.EnvVar(_sfxSchedulerLogsConfig) }

// SchedulerMaintenanceConfig returns the environment variable name for SCHEDULER_MAINTENANCE_CONFIG (brand-prefixed).
func SchedulerMaintenanceConfig() string { return brand.EnvVar(_sfxSchedulerMaintenanceConfig) }

// SchedulerMaxWallDuration returns the environment variable name for SCHEDULER_MAX_WALL_DURATION (brand-prefixed).
// Optional duration string for [time.ParseDuration] (e.g. "45m", "2h"); when set, the scheduler daemon cancels
// its main context after this wall time from process start (graceful shutdown). Unset: no wall limit.
func SchedulerMaxWallDuration() string { return brand.EnvVar(_sfxSchedulerMaxWallDuration) }

// SchedulerTriggeredPoolSize returns the environment variable name for SCHEDULER_TRIGGERED_POOL_SIZE (brand-prefixed).
func SchedulerTriggeredPoolSize() string { return brand.EnvVar(_sfxSchedulerTriggeredPoolSize) }

// SessionID returns the environment variable name for SESSION_ID (brand-prefixed).
func SessionID() string { return brand.EnvVar(_sfxSessionID) }

// SkipDeleteAudit returns the environment variable name for SKIP_DELETE_AUDIT (brand-prefixed).
func SkipDeleteAudit() string { return brand.EnvVar(_sfxSkipDeleteAudit) }

// SkipSpecSchemaValidation returns the environment variable name for SKIP_SPEC_SCHEMA_VALIDATION (brand-prefixed).
func SkipSpecSchemaValidation() string { return brand.EnvVar(_sfxSkipSpecSchemaValidation) }

// SpecializationTier returns the environment variable name for SPECIALIZATION_TIER (brand-prefixed).
// Defines the cellular specialization of the node (neuron, muscle, heart, lung).
func SpecializationTier() string { return brand.EnvVar(_sfxSpecializationTier) }

// StableBinaryPath returns the environment variable name for STABLE_BINARY_PATH (brand-prefixed).
func StableBinaryPath() string { return brand.EnvVar(_sfxStableBinaryPath) }

// StreamDeltaFieldsConfig returns the environment variable name for STREAM_DELTA_FIELDS_CONFIG (brand-prefixed).
func StreamDeltaFieldsConfig() string { return brand.EnvVar(_sfxStreamDeltaFieldsConfig) }

// StreamStorageEnabled returns the environment variable name for STREAM_STORAGE_ENABLED (brand-prefixed).
func StreamStorageEnabled() string { return brand.EnvVar(_sfxStreamStorageEnabled) }

// TaskArtifacts returns the environment variable name for TASK_ARTIFACTS (brand-prefixed).
// Comma-separated list of target artifact/package paths for agent task validation.
func TaskArtifacts() string { return brand.EnvVar(_sfxTaskArtifacts) }

// TableMaxRows returns the environment variable name for TABLE_MAX_ROWS (brand-prefixed).
func TableMaxRows() string { return brand.EnvVar(_sfxTableMaxRows) }

// TestCLIBinary returns the environment variable name for TEST_CLI_BINARY (brand-prefixed).
func TestCLIBinary() string { return brand.EnvVar(_sfxTestCLIBinary) }

// TestDataDir returns the environment variable name for TEST_DATA_DIR (brand-prefixed).
func TestDataDir() string { return brand.EnvVar(_sfxTestDataDir) }

// TestMetricsRecording returns the environment variable name for TEST_METRICS_RECORDING (brand-prefixed).
func TestMetricsRecording() string { return brand.EnvVar(_sfxTestMetricsRecording) }

// TestMode returns the environment variable name for TEST_MODE (brand-prefixed).
func TestMode() string { return brand.EnvVar(_sfxTestMode) }

// TestRoot returns the environment variable name for TEST_ROOT (brand-prefixed).
//
// This names a *location*: the project root to resolve against. Do not use it to express the
// *permission* to do something (see AllowForegroundGoTest). Conflating the two has cost this repo
// twice — once as falsely-green delete guards (see storage.denyCoreKernelHardDelete) and once as an
// agent guard whose own bypass advice relocated the CLI's project root, so following the advice
// broke every later command in that shell with an unrelated-looking account schema error.
func TestRoot() string { return brand.EnvVar(_sfxTestRoot) }

// AllowForegroundGoTest returns the environment variable name for ALLOW_FOREGROUND_GO_TEST
// (brand-prefixed). It grants exactly one thing: permission to run `go test` in the foreground
// without the agent guard panicking. It changes no paths and no behavior, which is the point —
// it is safe to export into a long-lived shell.
func AllowForegroundGoTest() string { return brand.EnvVar(_sfxAllowForegroundGoTest) }

// TestVerbose returns the environment variable name for TEST_VERBOSE (brand-prefixed).
func TestVerbose() string { return brand.EnvVar(_sfxTestVerbose) }

// UpdateHelpGolden returns the environment variable name for UPDATE_HELP_GOLDEN (brand-prefixed).
func UpdateHelpGolden() string { return brand.EnvVar(_sfxUpdateHelpGolden) }

// StorageMode returns the environment variable name for STORAGE_MODE (brand-prefixed).
// Values: "file" (default SSOT), "file+projection" (target), "hybrid_legacy" (transitional dual-write), "graph_only".
func StorageMode() string { return brand.EnvVar(_sfxStorageMode) }

// StorageModeHybridLegacy returns the environment variable name for STORAGE_MODE_HYBRID_LEGACY (brand-prefixed).
// Set to "1", "true", or "yes" to opt into legacy dual-write behavior when graph is enabled.
func StorageModeHybridLegacy() string { return brand.EnvVar(_sfxStorageModeHybridLegacy) }

// ProjectionFailClosed returns the env name for PROJECTION_FAIL_CLOSED (brand-prefixed).
// When "1"/"true"/"yes", FileFirstProjectionStorage returns projection errors after file SSOT succeeds.
func ProjectionFailClosed() string { return brand.EnvVar(_sfxProjectionFailClosed) }

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
	_rawZqkGraphEnabledLegacy    = "ZQK_GRAPH_ENABLED"
)

// AutofixBatchChunkSize returns the env var for autofix transactional chunk size (unprefixed; scheduler/automation contract).
func AutofixBatchChunkSize() string { return _rawAutofixBatchChunkSize }

// AutofixGlossaryMaxCreate returns the env var capping glossary term creation after autofix (unprefixed).
func AutofixGlossaryMaxCreate() string { return _rawAutofixGlossaryMaxCreate }

// POSIXHome returns the conventional HOME environment variable name.
func POSIXHome() string { return _rawEnvHome }

// POSIXPath returns the conventional PATH environment variable name.
func POSIXPath() string { return _rawEnvPath }

// POSIXUser returns the conventional USER environment variable name (Unix).
func POSIXUser() string { return _rawEnvUser }

// POSIXUsername returns the conventional USERNAME environment variable name (Windows).
func POSIXUsername() string { return _rawEnvUsername }

// ZqkGraphEnabledLegacy returns the legacy ZQK_GRAPH_ENABLED toggle (tests / older tooling).
func ZqkGraphEnabledLegacy() string { return _rawZqkGraphEnabledLegacy }

const _sfxSchedulerDaemonMode = "SCHEDULER_DAEMON_MODE"

func SchedulerDaemonMode() string { return brand.EnvVar(_sfxSchedulerDaemonMode) }

const _sfxDemoMode = "DEMO_MODE"
const _sfxEnableAmbientWatcher = "ENABLE_AMBIENT_WATCHER"
const _sfxFalAPIKey = "FAL_API_KEY"
const _sfxMubertCompanyID = "MUBERT_COMPANY_ID"
const _sfxMubertLicToken = "MUBERT_LIC_TOKEN"
const _sfxWatchdogTimeout = "WATCHDOG_TIMEOUT"

func WatchdogTimeout() string { return brand.EnvVar(_sfxWatchdogTimeout) }

const _sfxZqkShimBypassPolCode009 = "ZQK_SHIM_BYPASS_POLCODE009"

func ZqkShimBypassPolCode009() string { return brand.EnvVar(_sfxZqkShimBypassPolCode009) }

const _sfxBreakGlassReason = "BREAK_GLASS_REASON"

// BreakGlassReason returns the environment variable name for BREAK_GLASS_REASON (brand-prefixed, e.g. ZQK_BREAK_GLASS_REASON).
func BreakGlassReason() string { return brand.EnvVar(_sfxBreakGlassReason) }

// DemoMode returns the environment variable name for DEMO_MODE (brand-prefixed).
func DemoMode() string { return brand.EnvVar(_sfxDemoMode) }

// EnableAmbientWatcher returns the environment variable name for ENABLE_AMBIENT_WATCHER (brand-prefixed).
func EnableAmbientWatcher() string { return brand.EnvVar(_sfxEnableAmbientWatcher) }

// FalAPIKey returns the environment variable name for FAL_API_KEY (brand-prefixed).
func FalAPIKey() string { return brand.EnvVar(_sfxFalAPIKey) }

// MubertCompanyID returns the environment variable name for MUBERT_COMPANY_ID (brand-prefixed).
func MubertCompanyID() string { return brand.EnvVar(_sfxMubertCompanyID) }

// MubertLicToken returns the environment variable name for MUBERT_LIC_TOKEN (brand-prefixed).
func MubertLicToken() string { return brand.EnvVar(_sfxMubertLicToken) }

const _sfxTestBypassAuth = "TEST_BYPASS_AUTH"
const _sfxTestMockSuccess = "TEST_MOCK_SUCCESS"
const _sfxTestMockFailure = "TEST_MOCK_FAILURE"
const _sfxFileFallback = "FILE_FALLBACK"

// TestBypassAuth returns the environment variable name for TEST_BYPASS_AUTH (brand-prefixed).
func TestBypassAuth() string { return brand.EnvVar(_sfxTestBypassAuth) }

// TestMockSuccess returns the environment variable name for TEST_MOCK_SUCCESS (brand-prefixed).
func TestMockSuccess() string { return brand.EnvVar(_sfxTestMockSuccess) }

// TestMockFailure returns the environment variable name for TEST_MOCK_FAILURE (brand-prefixed).
func TestMockFailure() string { return brand.EnvVar(_sfxTestMockFailure) }

// FileFallback returns the environment variable name for FILE_FALLBACK (brand-prefixed).
func FileFallback() string { return brand.EnvVar(_sfxFileFallback) }

const _sfxLLMProvider = "LLM_PROVIDER"
const _sfxGeminiAPIKey = "GEMINI_API_KEY"

// LLMProvider returns the environment variable name for LLM_PROVIDER (brand-prefixed).
func LLMProvider() string { return brand.EnvVar(_sfxLLMProvider) }

// GeminiAPIKey returns the environment variable name for GEMINI_API_KEY (brand-prefixed).
func GeminiAPIKey() string { return brand.EnvVar(_sfxGeminiAPIKey) }

const _sfxTestSkipValidation = "TEST_SKIP_VALIDATION"
const _sfxTestAllowCASFallthrough = "TEST_ALLOW_CAS_FALLTHROUGH"
const _sfxPrivilegedWriterSocket = "PRIVILEGED_WRITER_SOCKET"

func TestSkipValidation() string { return brand.EnvVar(_sfxTestSkipValidation) }

// TestAllowCASFallthrough returns the env name for TEST_ALLOW_CAS_FALLTHROUGH (brand-prefixed).
// When set to "1", CAS create/update/delete may write locally if PrivilegedWriter is unreachable.
func TestAllowCASFallthrough() string { return brand.EnvVar(_sfxTestAllowCASFallthrough) }

// PrivilegedWriterSocket returns the env name for PRIVILEGED_WRITER_SOCKET (brand-prefixed).
// When set, overrides the default UNIX socket path for the PrivilegedWriter helper.
func PrivilegedWriterSocket() string { return brand.EnvVar(_sfxPrivilegedWriterSocket) }

const _sfxIsParentZqk = "IS_PARENT_ZQK"

// IsParentZqk returns the environment variable name for IS_PARENT_ZQK (brand-prefixed).
func IsParentZqk() string { return brand.EnvVar(_sfxIsParentZqk) }

const _sfxIsDaemon = "IS_DAEMON"

// IsDaemon returns the env name for IS_DAEMON (brand-prefixed). Set to "1" by the privileged
// writer daemon so in-process storage writes locally (no self-dial), skips write-behind WAL
// replay, and takes the async hash-registry save path.
// TRACK: BLI-CEF-R20-SINGLE-WRITER-BLI-001 — remove when: PW is a thin CAS endpoint, not a second write-behind owner
func IsDaemon() string { return brand.EnvVar(_sfxIsDaemon) }

const _sfxPrototypeAccountRefs = "PROTOTYPE_ACCOUNT_REFS"

// PrototypeAccountRefs returns the env name for PROTOTYPE_ACCOUNT_REFS (brand-prefixed).
// Comma-separated account refs that must be rejected as prototype/test accounts in production
// (see pkg/authcred.RegisterPrototypeAccountRefs).
func PrototypeAccountRefs() string { return brand.EnvVar(_sfxPrototypeAccountRefs) }

const _sfxTDDDiffTarget = "TDD_DIFF_TARGET"

// TDDDiffTarget returns the environment variable name for TDD_DIFF_TARGET (brand-prefixed).
func TDDDiffTarget() string { return brand.EnvVar(_sfxTDDDiffTarget) }

const _sfxEnableStudioPackTools = "ENABLE_STUDIO_PACK_TOOLS"

// EnableStudioPackTools returns the environment variable name for ENABLE_STUDIO_PACK_TOOLS (brand-prefixed).
func EnableStudioPackTools() string { return brand.EnvVar(_sfxEnableStudioPackTools) }

const _sfxStudioDogfood = "STUDIO_DOGFOOD"

// StudioDogfood returns the environment variable name for STUDIO_DOGFOOD (brand-prefixed).
func StudioDogfood() string { return brand.EnvVar(_sfxStudioDogfood) }

const _sfxTestBypassGitevidence = "TEST_BYPASS_GITEVIDENCE"

// TestBypassGitevidence returns the environment variable name for TEST_BYPASS_GITEVIDENCE (brand-prefixed).
func TestBypassGitevidence() string { return brand.EnvVar(_sfxTestBypassGitevidence) }

const _sfxSkipStateCommit = "SKIP_STATE_COMMIT"

// SkipStateCommit returns the environment variable name for SKIP_STATE_COMMIT (brand-prefixed).
func SkipStateCommit() string { return brand.EnvVar(_sfxSkipStateCommit) }
