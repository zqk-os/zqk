package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestConfigLoad_DefaultsFile verifies that config/zqk.yaml is loaded and parsed.
func TestConfigLoad_DefaultsFile(t *testing.T) {
	resetForTesting()
	defer resetForTesting()

	cfg := loadConfig()
	if cfg == nil {
		t.Fatal("loadConfig returned nil")
	}
}

// TestConfigLoad_YAMLRoundTrip verifies the struct can unmarshal all sections.
func TestConfigLoad_YAMLRoundTrip(t *testing.T) {
	yamlData := `
system:
  storage_mode: "file"
  storage_mode_hybrid_legacy: true
  stream_storage_enabled: false
  projection_fail_closed: true
  max_os_threads: 8
  max_object_yaml_io: 128
  host_cpu_backpressure: true
  hostload_disable: false
  demo_mode: true
  specialization_tier: "neuron"
  skip_delete_audit: false
scheduler:
  admission_timeout: "90s"
  dispatch_resource_wait_max: "45s"
  max_wall_duration: "2h"
  goroutine_cap: 100
  default_package_concurrency: 8
  triggered_pool_size: 16
  immediate_load_batch_size: 50
  immediate_load_batch_pause: "1s"
  daemon_mode: true
  daemon_bin: "/usr/local/bin/zqk-scheduler"
  logs_config: "/etc/zqk/logs.yaml"
  maintenance_config: "/etc/zqk/maintenance.yaml"
  cvs_ledger_max_failures: 25
  aggregation_metric_creation_timeout: "30s"
llm:
  provider: "openai"
  chat_model: "gpt-4"
  context_window_size: 128000
  temperature: 0.7
  top_p: 0.9
  embed_model: "text-embedding-3-small"
  base_url: "http://localhost:11434/v1"
  openai_base_url: "https://api.openai.com/v1"
  fal_base_url: "https://fal.run"
  mubert_base_url: "https://api.mubert.com"
  qwen_base_url: "http://localhost:11434/v1"
  trace: true
storage:
  graph_enabled: true
  mock_graph: false
  legacy_mock_graph: false
  admin_graph_enabled: true
  admin_mock_graph: false
  high_volume_cache_build_workers: 4
  high_volume_cache_kind_parallelism: 2
  bulk_bench_size: 1000
  table_max_rows: 200
  rollback_retain_count: 10
  rollback_retain_duration: "336h"
  rollback_capture_disabled: false
  list_count_max_concurrent: 8
  list_read_workers: 4
  stream_delta_fields_config: "default"
maintenance:
  autofix_batch_chunk_size: 100
  autofix_glossary_max_create: 20
  metrics_chunk_retention_days: 60
  retention_tolerance_config: "strict"
  cache_diagnostic_enabled: true
  cache_diagnostic_objects: "goal,requirement"
  cache_diagnostic_prefixes: "GOA-,REQ-"
swarm:
  max_steps: 50
  watchdog_timeout: "10m"
validation:
  disable_criteria_auto_validate: true
  skip_spec_schema_validation: false
  skip_state_commit: false
  debug_validation_object_ids: "OBJ-123,OBJ-456"
testing:
  mock_success: false
  mock_failure: false
  bypass_auth: true
  skip_validation: false
  metrics_recording: true
  verbose: true
  mode: true
  crud_baseline_count: 500
  crud_heap_profile: true
  crud_profile: true
  run_perf_tests: true
  stress_real_root: false
  populate_scenario_test: false
  bypass_gitevidence: true
  enable_bootstrap_crud_tests: true
  enable_spec_cell_integration_tests: true
  enable_spec_cell_req019_validate: true
  enable_cas_migration_scenario_test: false
  enable_cli_scenario_tests: true
  enable_hash_registry_coordinator_tests: true
  enable_ioqueue_shutdown_tests: false
  enable_public_candidate_test: true
  enable_migrate_legacy_to_stream_integration_test: false
  enable_studio_pack_tools: true
  studio_dogfood: true
  enable_ambient_watcher: true
  tdd_diff_target: "main"
paths:
  mcp_config_path: "/etc/zqk/mcp.json"
  test_data_dir: "/tmp/test-data"
  task_artifacts: "pkg/foo,cmd/bar"
  llm_trace_dir: "/tmp/llm-traces"
  diagnostics_dir: "/tmp/diagnostics"
  stable_binary_path: "/usr/local/bin/zqk"
  ffmpeg_worker_endpoint: "http://localhost:8080"
cli:
  update_help_golden: true
kernel_state:
  project_root: "/home/user/project"
mcp:
  run_deadlock_reproduction: true
logging:
  scheduler_logs_config: "/etc/zqk/scheduler-logs.yaml"
diagnostics:
  pprof: true
  pprof_port: 6061
  thread_stack_skip_min_goroutines: 100
ide:
  paste_prefix_steps: "escape,cmd_l"
  paste_app: "VSCode"
`
	var cfg ZqkConfig
	if err := yaml.Unmarshal([]byte(yamlData), &cfg); err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	// System
	assertStr(t, "system.storage_mode", cfg.System.StorageMode, "file")
	assertBool(t, "system.storage_mode_hybrid_legacy", cfg.System.StorageModeHybridLegacy, true)
	assertBool(t, "system.projection_fail_closed", cfg.System.ProjectionFailClosed, true)
	assertInt(t, "system.max_os_threads", cfg.System.MaxOSThreads, 8)
	assertInt(t, "system.max_object_yaml_io", cfg.System.MaxObjectYAMLIO, 128)
	assertBool(t, "system.host_cpu_backpressure", cfg.System.HostCPUBackpressure, true)
	assertBool(t, "system.demo_mode", cfg.System.DemoMode, true)
	assertStr(t, "system.specialization_tier", cfg.System.SpecializationTier, "neuron")

	// Scheduler
	assertStr(t, "scheduler.admission_timeout", cfg.Scheduler.AdmissionTimeout, "90s")
	assertInt(t, "scheduler.goroutine_cap", cfg.Scheduler.GoroutineCap, 100)
	assertInt(t, "scheduler.default_package_concurrency", cfg.Scheduler.DefaultPackageConcurrency, 8)
	assertBool(t, "scheduler.daemon_mode", cfg.Scheduler.DaemonMode, true)
	assertStr(t, "scheduler.daemon_bin", cfg.Scheduler.DaemonBin, "/usr/local/bin/zqk-scheduler")
	assertStr(t, "scheduler.logs_config", cfg.Scheduler.LogsConfig, "/etc/zqk/logs.yaml")
	assertInt(t, "scheduler.cvs_ledger_max_failures", cfg.Scheduler.CVSLedgerMaxFailures, 25)

	// LLM
	assertStr(t, "llm.provider", cfg.LLM.Provider, "openai")
	assertStr(t, "llm.chat_model", cfg.LLM.ChatModel, "gpt-4")
	assertInt(t, "llm.context_window_size", cfg.LLM.ContextWindowSize, 128000)
	assertFloat(t, "llm.temperature", cfg.LLM.Temperature, 0.7)
	assertFloat(t, "llm.top_p", cfg.LLM.TopP, 0.9)
	assertBool(t, "llm.trace", cfg.LLM.Trace, true)

	// Storage
	assertBool(t, "storage.graph_enabled", cfg.Storage.GraphEnabled, true)
	assertBool(t, "storage.admin_graph_enabled", cfg.Storage.AdminGraphEnabled, true)
	assertInt(t, "storage.table_max_rows", cfg.Storage.TableMaxRows, 200)
	assertInt(t, "storage.rollback_retain_count", cfg.Storage.RollbackRetainCount, 10)
	assertInt(t, "storage.list_count_max_concurrent", cfg.Storage.ListCountMaxConcurrent, 8)
	assertInt(t, "storage.list_read_workers", cfg.Storage.ListReadWorkers, 4)
	assertStr(t, "storage.stream_delta_fields_config", cfg.Storage.StreamDeltaFieldsConfig, "default")

	// Maintenance
	assertInt(t, "maintenance.autofix_batch_chunk_size", cfg.Maintenance.AutofixBatchChunkSize, 100)
	assertInt(t, "maintenance.metrics_chunk_retention_days", cfg.Maintenance.MetricsChunkRetentionDays, 60)
	assertBool(t, "maintenance.cache_diagnostic_enabled", cfg.Maintenance.CacheDiagnosticEnabled, true)

	// Swarm
	assertInt(t, "swarm.max_steps", cfg.Swarm.MaxSteps, 50)
	assertStr(t, "swarm.watchdog_timeout", cfg.Swarm.WatchdogTimeout, "10m")

	// Validation
	assertBool(t, "validation.disable_criteria_auto_validate", cfg.Validation.DisableCriteriaAutoValidate, true)
	assertStr(t, "validation.debug_validation_object_ids", cfg.Validation.DebugValidationObjectIDs, "OBJ-123,OBJ-456")

	// Testing (spot checks)
	assertBool(t, "testing.bypass_auth", cfg.Testing.BypassAuth, true)
	assertBool(t, "testing.verbose", cfg.Testing.Verbose, true)
	assertInt(t, "testing.crud_baseline_count", cfg.Testing.CRUDBaselineCount, 500)
	assertBool(t, "testing.crud_heap_profile", cfg.Testing.CRUDHeapProfile, true)
	assertBool(t, "testing.enable_bootstrap_crud_tests", cfg.Testing.EnableBootstrapCRUDTests, true)
	assertBool(t, "testing.enable_cli_scenario_tests", cfg.Testing.EnableCLIScenarioTests, true)
	assertBool(t, "testing.enable_studio_pack_tools", cfg.Testing.EnableStudioPackTools, true)
	assertBool(t, "testing.enable_ambient_watcher", cfg.Testing.EnableAmbientWatcher, true)
	assertStr(t, "testing.tdd_diff_target", cfg.Testing.TDDDiffTarget, "main")

	// Paths
	assertStr(t, "paths.stable_binary_path", cfg.Paths.StableBinaryPath, "/usr/local/bin/zqk")
	assertStr(t, "paths.ffmpeg_worker_endpoint", cfg.Paths.FFMPEGWorkerEndpoint, "http://localhost:8080")
	assertStr(t, "paths.diagnostics_dir", cfg.Paths.DiagnosticsDir, "/tmp/diagnostics")

	// Diagnostics (new section)
	assertBool(t, "diagnostics.pprof", cfg.Diagnostics.Pprof, true)
	assertInt(t, "diagnostics.pprof_port", cfg.Diagnostics.PprofPort, 6061)
	assertInt(t, "diagnostics.thread_stack_skip_min_goroutines", cfg.Diagnostics.ThreadStackSkipMinGoroutines, 100)

	// IDE (new section)
	assertStr(t, "ide.paste_prefix_steps", cfg.IDE.PastePrefixSteps, "escape,cmd_l")
	assertStr(t, "ide.paste_app", cfg.IDE.PasteApp, "VSCode")

	// MCP
	assertBool(t, "mcp.run_deadlock_reproduction", cfg.MCP.RunDeadlockReproduction, true)

	// CLI
	assertBool(t, "cli.update_help_golden", cfg.CLI.UpdateHelpGolden, true)
}

// TestConfigLoad_LocalOverrides verifies that zqk-local.yaml overrides zqk.yaml.
func TestConfigLoad_LocalOverrides(t *testing.T) {
	tmpDir := t.TempDir()

	// Write defaults
	defaults := `
system:
  max_os_threads: 4
  demo_mode: false
storage:
  table_max_rows: 100
diagnostics:
  pprof: false
  pprof_port: 6060
`
	configDir := filepath.Join(tmpDir, paths.ConfigDir)
	if err := fileutil.EnsureDir(configDir); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(filepath.Join(configDir, paths.ZqkConfigFileName), []byte(defaults)); err != nil {
		t.Fatal(err)
	}

	// Write local overrides
	local := `
system:
  max_os_threads: 16
  demo_mode: true
diagnostics:
  pprof: true
  pprof_port: 6061
`
	if err := fileutil.WriteStandardFile(filepath.Join(configDir, paths.ZqkLocalConfigFileName), []byte(local)); err != nil {
		t.Fatal(err)
	}

	// Change working directory so loadConfig finds the config/ dir
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origDir) }()

	resetForTesting()
	defer resetForTesting()

	cfg := loadConfig()

	// Overridden values
	assertInt(t, "system.max_os_threads (overridden)", cfg.System.MaxOSThreads, 16)
	assertBool(t, "system.demo_mode (overridden)", cfg.System.DemoMode, true)
	assertBool(t, "diagnostics.pprof (overridden)", cfg.Diagnostics.Pprof, true)
	assertInt(t, "diagnostics.pprof_port (overridden)", cfg.Diagnostics.PprofPort, 6061)

	// Non-overridden values preserved from defaults
	assertInt(t, "storage.table_max_rows (from defaults)", cfg.Storage.TableMaxRows, 100)
}

// TestAccessor_NewSections verifies accessors for the new Diagnostics and IDE sections.
func TestAccessor_NewSections(t *testing.T) {
	resetForTesting()
	defer resetForTesting()

	// Override Get to work without config file by setting globalConfig directly
	globalConfig = &ZqkConfig{}

	if DiagnosticsPprof().Safe() != false {
		t.Error("Expected DiagnosticsPprof().Safe() == false when not configured")
	}
	if DiagnosticsPprofPort().Safe() != 0 {
		t.Error("Expected DiagnosticsPprofPort().Safe() == 0 when not configured")
	}
	if DiagnosticsThreadStackSkipMinGoroutines().Safe() != 0 {
		t.Error("Expected DiagnosticsThreadStackSkipMinGoroutines().Safe() == 0 when not configured")
	}
	if IDEPastePrefixSteps().Safe() != "" {
		t.Error("Expected IDEPastePrefixSteps().Safe() == '' when not configured")
	}
	if IDEPasteApp().Safe() != "" {
		t.Error("Expected IDEPasteApp().Safe() == '' when not configured")
	}
}

// TestAccessor_NewSystemFields verifies accessors for new System fields.
func TestAccessor_NewSystemFields(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	globalConfig = &ZqkConfig{}

	if SystemHostCPUBackpressure().Safe() != false {
		t.Error("Expected SystemHostCPUBackpressure().Safe() == false")
	}
	if SystemHostloadDisable().Safe() != false {
		t.Error("Expected SystemHostloadDisable().Safe() == false")
	}
	if SystemDemoMode().Safe() != false {
		t.Error("Expected SystemDemoMode().Safe() == false")
	}
	if SystemSpecializationTier().Safe() != "" {
		t.Error("Expected SystemSpecializationTier().Safe() == ''")
	}
	if SystemSkipDeleteAudit().Safe() != false {
		t.Error("Expected SystemSkipDeleteAudit().Safe() == false")
	}
	if SystemStorageModeHybridLegacy().Safe() != false {
		t.Error("Expected SystemStorageModeHybridLegacy().Safe() == false")
	}
}

// TestAccessor_NewSchedulerFields verifies accessors for new Scheduler fields.
func TestAccessor_NewSchedulerFields(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	globalConfig = &ZqkConfig{}

	if SchedulerDaemonBin().Safe() != "" {
		t.Error("Expected SchedulerDaemonBin().Safe() == ''")
	}
	if SchedulerLogsConfigPath().Safe() != "" {
		t.Error("Expected SchedulerLogsConfigPath().Safe() == ''")
	}
	if SchedulerMaintenanceConfigPath().Safe() != "" {
		t.Error("Expected SchedulerMaintenanceConfigPath().Safe() == ''")
	}
	if SchedulerCVSLedgerMaxFailures().Safe() != 0 {
		t.Error("Expected SchedulerCVSLedgerMaxFailures().Safe() == 0")
	}
	if SchedulerAggregationMetricCreationTimeout().Safe() != "" {
		t.Error("Expected SchedulerAggregationMetricCreationTimeout().Safe() == ''")
	}
}

// TestAccessor_NewStorageFields verifies accessors for new Storage fields.
func TestAccessor_NewStorageFields(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	globalConfig = &ZqkConfig{}

	if StorageListCountMaxConcurrent().Safe() != 0 {
		t.Error("Expected StorageListCountMaxConcurrent().Safe() == 0")
	}
	if StorageListReadWorkers().Safe() != 0 {
		t.Error("Expected StorageListReadWorkers().Safe() == 0")
	}
	if StorageStreamDeltaFieldsConfig().Safe() != "" {
		t.Error("Expected StorageStreamDeltaFieldsConfig().Safe() == ''")
	}
}

// TestAccessor_NewLLMFields verifies accessors for new LLM fields.
func TestAccessor_NewLLMFields(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	globalConfig = &ZqkConfig{}

	if LLMTopP().Safe() != 0.0 {
		t.Error("Expected LLMTopP().Safe() == 0.0")
	}
	if LLMTrace().Safe() != false {
		t.Error("Expected LLMTrace().Safe() == false")
	}
}

// TestAccessor_NewTestingFields verifies accessors for new Testing fields.
func TestAccessor_NewTestingFields(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	globalConfig = &ZqkConfig{}

	boolAccessors := []struct {
		name string
		fn   func() Property[bool]
	}{
		{"CRUDHeapProfile", TestingCRUDHeapProfile},
		{"CRUDProfile", TestingCRUDProfile},
		{"RunPerfTests", TestingRunPerfTests},
		{"StressRealRoot", TestingStressRealRoot},
		{"PopulateScenarioTest", TestingPopulateScenarioTest},
		{"BypassGitevidence", TestingBypassGitevidence},
		{"EnableBootstrapCRUDTests", TestingEnableBootstrapCRUDTests},
		{"EnableSpecCellIntegrationTests", TestingEnableSpecCellIntegrationTests},
		{"EnableSpecCellREQ019Validate", TestingEnableSpecCellREQ019Validate},
		{"EnableCASMigrationScenarioTest", TestingEnableCASMigrationScenarioTest},
		{"EnableCLIScenarioTests", TestingEnableCLIScenarioTests},
		{"EnableHashRegistryCoordinatorTests", TestingEnableHashRegistryCoordinatorTests},
		{"EnableIOQueueShutdownTests", TestingEnableIOQueueShutdownTests},
		{"EnablePublicCandidateTest", TestingEnablePublicCandidateTest},
		{"EnableMigrateLegacyToStreamIntegrationTest", TestingEnableMigrateLegacyToStreamIntegrationTest},
		{"EnableStudioPackTools", TestingEnableStudioPackTools},
		{"StudioDogfood", TestingStudioDogfood},
		{"EnableAmbientWatcher", TestingEnableAmbientWatcher},
	}
	for _, tc := range boolAccessors {
		if tc.fn().Safe() != false {
			t.Errorf("Expected Testing.%s.Safe() == false when not configured", tc.name)
		}
	}
	if TestingTDDDiffTarget().Safe() != "" {
		t.Error("Expected TestingTDDDiffTarget().Safe() == '' when not configured")
	}
}

// TestAccessor_NewPathsFields verifies accessors for new Paths fields.
func TestAccessor_NewPathsFields(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	globalConfig = &ZqkConfig{}

	if PathsStableBinaryPath().Safe() != "" {
		t.Error("Expected PathsStableBinaryPath().Safe() == ''")
	}
	if PathsFFMPEGWorkerEndpoint().Safe() != "" {
		t.Error("Expected PathsFFMPEGWorkerEndpoint().Safe() == ''")
	}
}

// TestAccessor_OrDefault_WithConfiguredValue verifies OrDefault returns the configured value.
func TestAccessor_OrDefault_WithConfiguredValue(t *testing.T) {
	resetForTesting()
	defer resetForTesting()

	port := 6061
	globalConfig = &ZqkConfig{}
	globalConfig.Diagnostics.PprofPort = &port

	got := DiagnosticsPprofPort().OrDefault(6060)
	if got != 6061 {
		t.Errorf("Expected OrDefault to return configured value 6061, got %d", got)
	}
}

// TestAccessor_Required_Panics verifies Required panics when config is not set.
func TestAccessor_Required_Panics(t *testing.T) {
	resetForTesting()
	defer resetForTesting()
	globalConfig = &ZqkConfig{}

	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected Required() to panic on nil diagnostics.pprof_port")
		}
	}()
	_ = DiagnosticsPprofPort().Required()
}

// --- Test helpers ---

func assertStr(t *testing.T, field string, ptr *string, want string) {
	t.Helper()
	if ptr == nil {
		t.Errorf("%s: expected %q, got nil", field, want)
		return
	}
	if *ptr != want {
		t.Errorf("%s: expected %q, got %q", field, want, *ptr)
	}
}

func assertBool(t *testing.T, field string, ptr *bool, want bool) {
	t.Helper()
	if ptr == nil {
		t.Errorf("%s: expected %v, got nil", field, want)
		return
	}
	if *ptr != want {
		t.Errorf("%s: expected %v, got %v", field, want, *ptr)
	}
}

func assertInt(t *testing.T, field string, ptr *int, want int) {
	t.Helper()
	if ptr == nil {
		t.Errorf("%s: expected %d, got nil", field, want)
		return
	}
	if *ptr != want {
		t.Errorf("%s: expected %d, got %d", field, want, *ptr)
	}
}

func assertFloat(t *testing.T, field string, ptr *float64, want float64) {
	t.Helper()
	if ptr == nil {
		t.Errorf("%s: expected %f, got nil", field, want)
		return
	}
	if *ptr != want {
		t.Errorf("%s: expected %f, got %f", field, want, *ptr)
	}
}

func TestConfigLoad_SubdirectoryRootResolution(t *testing.T) {
	resetForTesting()
	defer resetForTesting()

	cfg := loadConfig()
	if cfg == nil {
		t.Fatal("loadConfig returned nil from subdirectory")
	}
	if cfg.System.StorageMode == nil {
		t.Fatal("expected system.storage_mode to be loaded from root config/zqk.yaml, got nil")
	}
}
