package config

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type ZqkConfig struct {
	System struct {
		StorageMode             *string `yaml:"storage_mode"`
		StorageModeHybridLegacy *bool   `yaml:"storage_mode_hybrid_legacy"`
		StreamStorageEnabled    *bool   `yaml:"stream_storage_enabled"`
		ProjectionFailClosed    *bool   `yaml:"projection_fail_closed"`
		MaxOSThreads            *int    `yaml:"max_os_threads"`
		MaxObjectYAMLIO         *int    `yaml:"max_object_yaml_io"`
		HostCPUBackpressure     *bool   `yaml:"host_cpu_backpressure"`
		HostloadDisable         *bool   `yaml:"hostload_disable"`
		DemoMode                *bool   `yaml:"demo_mode"`
		SpecializationTier      *string `yaml:"specialization_tier"`
		SkipDeleteAudit         *bool   `yaml:"skip_delete_audit"`
	} `yaml:"system"`
	Scheduler struct {
		AdmissionTimeout                 *string `yaml:"admission_timeout"`
		DispatchResourceWaitMax          *string `yaml:"dispatch_resource_wait_max"`
		MaxWallDuration                  *string `yaml:"max_wall_duration"`
		GoroutineCap                     *int    `yaml:"goroutine_cap"`
		DefaultPackageConcurrency        *int    `yaml:"default_package_concurrency"`
		TriggeredPoolSize                *int    `yaml:"triggered_pool_size"`
		ImmediateLoadBatchSize           *int    `yaml:"immediate_load_batch_size"`
		ImmediateLoadBatchPause          *string `yaml:"immediate_load_batch_pause"`
		DaemonMode                       *bool   `yaml:"daemon_mode"`
		DaemonBin                        *string `yaml:"daemon_bin"`
		LogsConfig                       *string `yaml:"logs_config"`
		MaintenanceConfig                *string `yaml:"maintenance_config"`
		CVSLedgerMaxFailures             *int    `yaml:"cvs_ledger_max_failures"`
		AggregationMetricCreationTimeout *string `yaml:"aggregation_metric_creation_timeout"`
	} `yaml:"scheduler"`
	LLM struct {
		Provider          *string  `yaml:"provider"`
		ChatModel         *string  `yaml:"chat_model"`
		ContextWindowSize *int     `yaml:"context_window_size"`
		Temperature       *float64 `yaml:"temperature"`
		TopP              *float64 `yaml:"top_p"`
		EmbedModel        *string  `yaml:"embed_model"`
		BaseURL           *string  `yaml:"base_url"`
		OpenAIBaseURL     *string  `yaml:"openai_base_url"`
		FalBaseURL        *string  `yaml:"fal_base_url"`
		MubertBaseURL     *string  `yaml:"mubert_base_url"`
		QwenBaseURL       *string  `yaml:"qwen_base_url"`
		Trace             *bool    `yaml:"trace"`
	} `yaml:"llm"`
	Storage struct {
		GraphEnabled                   *bool   `yaml:"graph_enabled"`
		MockGraph                      *bool   `yaml:"mock_graph"`
		LegacyMockGraph                *bool   `yaml:"legacy_mock_graph"`
		AdminGraphEnabled              *bool   `yaml:"admin_graph_enabled"`
		AdminMockGraph                 *bool   `yaml:"admin_mock_graph"`
		HighVolumeCacheBuildWorkers    *int    `yaml:"high_volume_cache_build_workers"`
		HighVolumeCacheKindParallelism *int    `yaml:"high_volume_cache_kind_parallelism"`
		BulkBenchSize                  *int    `yaml:"bulk_bench_size"`
		TableMaxRows                   *int    `yaml:"table_max_rows"`
		RollbackRetainCount            *int    `yaml:"rollback_retain_count"`
		RollbackRetainDuration         *string `yaml:"rollback_retain_duration"`
		RollbackCaptureDisabled        *bool   `yaml:"rollback_capture_disabled"`
		ListCountMaxConcurrent         *int    `yaml:"list_count_max_concurrent"`
		ListReadWorkers                *int    `yaml:"list_read_workers"`
		StreamDeltaFieldsConfig        *string `yaml:"stream_delta_fields_config"`
	} `yaml:"storage"`
	Maintenance struct {
		AutofixBatchChunkSize     *int    `yaml:"autofix_batch_chunk_size"`
		AutofixGlossaryMaxCreate  *int    `yaml:"autofix_glossary_max_create"`
		MetricsChunkRetentionDays *int    `yaml:"metrics_chunk_retention_days"`
		RetentionToleranceConfig  *string `yaml:"retention_tolerance_config"`
		CacheDiagnosticEnabled    *bool   `yaml:"cache_diagnostic_enabled"`
		CacheDiagnosticObjects    *string `yaml:"cache_diagnostic_objects"`
		CacheDiagnosticPrefixes   *string `yaml:"cache_diagnostic_prefixes"`
	} `yaml:"maintenance"`
	Swarm struct {
		MaxSteps        *int    `yaml:"max_steps"`
		WatchdogTimeout *string `yaml:"watchdog_timeout"`
	} `yaml:"swarm"`
	Validation struct {
		DisableCriteriaAutoValidate *bool   `yaml:"disable_criteria_auto_validate"`
		SkipSpecSchemaValidation    *bool   `yaml:"skip_spec_schema_validation"`
		SkipStateCommit             *bool   `yaml:"skip_state_commit"`
		DebugValidationObjectIDs    *string `yaml:"debug_validation_object_ids"`
	} `yaml:"validation"`
	Testing struct {
		MockSuccess                                *bool   `yaml:"mock_success"`
		MockFailure                                *bool   `yaml:"mock_failure"`
		BypassAuth                                 *bool   `yaml:"bypass_auth"`
		SkipValidation                             *bool   `yaml:"skip_validation"`
		MetricsRecording                           *bool   `yaml:"metrics_recording"`
		Verbose                                    *bool   `yaml:"verbose"`
		Mode                                       *bool   `yaml:"mode"`
		CRUDBaselineCount                          *int    `yaml:"crud_baseline_count"`
		CRUDHeapProfile                            *bool   `yaml:"crud_heap_profile"`
		CRUDProfile                                *bool   `yaml:"crud_profile"`
		RunPerfTests                               *bool   `yaml:"run_perf_tests"`
		StressRealRoot                             *bool   `yaml:"stress_real_root"`
		PopulateScenarioTest                       *bool   `yaml:"populate_scenario_test"`
		BypassGitevidence                          *bool   `yaml:"bypass_gitevidence"`
		EnableBootstrapCRUDTests                   *bool   `yaml:"enable_bootstrap_crud_tests"`
		EnableSpecCellIntegrationTests             *bool   `yaml:"enable_spec_cell_integration_tests"`
		EnableSpecCellREQ019Validate               *bool   `yaml:"enable_spec_cell_req019_validate"`
		EnableCASMigrationScenarioTest             *bool   `yaml:"enable_cas_migration_scenario_test"`
		EnableCLIScenarioTests                     *bool   `yaml:"enable_cli_scenario_tests"`
		EnableHashRegistryCoordinatorTests         *bool   `yaml:"enable_hash_registry_coordinator_tests"`
		EnableIOQueueShutdownTests                 *bool   `yaml:"enable_ioqueue_shutdown_tests"`
		EnablePublicCandidateTest                  *bool   `yaml:"enable_public_candidate_test"`
		EnableMigrateLegacyToStreamIntegrationTest *bool   `yaml:"enable_migrate_legacy_to_stream_integration_test"`
		EnableStudioPackTools                      *bool   `yaml:"enable_studio_pack_tools"`
		StudioDogfood                              *bool   `yaml:"studio_dogfood"`
		EnableAmbientWatcher                       *bool   `yaml:"enable_ambient_watcher"`
		TDDDiffTarget                              *string `yaml:"tdd_diff_target"`
	} `yaml:"testing"`
	Paths struct {
		ProjectRoot          *string           `yaml:"project_root"`
		MCPConfigPath        *string           `yaml:"mcp_config_path"`
		TestDataDir          *string           `yaml:"test_data_dir"`
		TaskArtifacts        *string           `yaml:"task_artifacts"`
		LLMTraceDir          *string           `yaml:"llm_trace_dir"`
		DiagnosticsDir       *string           `yaml:"diagnostics_dir"`
		StableBinaryPath     *string           `yaml:"stable_binary_path"`
		FFMPEGWorkerEndpoint *string           `yaml:"ffmpeg_worker_endpoint"`
		StalenessCheckDirs   []string          `yaml:"staleness_check_dirs"`
		Aliases              map[string]string `yaml:"aliases"`
	} `yaml:"paths"`
	CLI struct {
		UpdateHelpGolden *bool   `yaml:"update_help_golden"`
		DefaultContext   *string `yaml:"default_context"`
		BinaryPath       *string `yaml:"binary_path"`
	} `yaml:"cli"`
	KernelState struct {
		ProjectRoot        *string `yaml:"project_root"`
		SnapshotBackupDir  *string `yaml:"snapshot_backup_dir"`
		SnapshotBackupKeep *int    `yaml:"snapshot_backup_keep"`
	} `yaml:"kernel_state"`
	MCP struct {
		RunDeadlockReproduction *bool `yaml:"run_deadlock_reproduction"`
	} `yaml:"mcp"`
	Logging struct {
		ErrorLogOutput      *string `yaml:"error_log_output"`
		Level               *string `yaml:"level"`
		SchedulerLogsConfig *string `yaml:"scheduler_logs_config"`
	} `yaml:"logging"`
	Diagnostics struct {
		Pprof                        *bool `yaml:"pprof"`
		PprofPort                    *int  `yaml:"pprof_port"`
		ThreadStackSkipMinGoroutines *int  `yaml:"thread_stack_skip_min_goroutines"`
	} `yaml:"diagnostics"`
	IDE struct {
		PastePrefixSteps *string `yaml:"paste_prefix_steps"`
		PasteApp         *string `yaml:"paste_app"`
	} `yaml:"ide"`
}

var (
	globalConfig *ZqkConfig
	configOnce   sync.Once
	rootConfigs  stampmemo.Table[*ZqkConfig] // keyed by project root; stamp is the config files
)

func Get() *ZqkConfig {
	if globalConfig != nil {
		return globalConfig
	}
	return loadConfig()
}

func loadConfig() *ZqkConfig {
	wd, err := fileutil.Getwd()
	if err != nil {
		return &ZqkConfig{}
	}

	root := wd
	if !fileutil.Exists(filepath.Join(wd, "config", "zqk.yaml")) && !fileutil.Exists(filepath.Join(wd, "zqk.yaml")) {
		if r := paths.ResolveProjectRoot(wd); r != "" {
			root = r
		} else if r := paths.FindNearestProjectRoot(wd); r != "" {
			root = r
		}
	}

	return LoadForRoot(root)
}

// LoadForRoot loads configuration originating from config/** for a specific project root,
// applying defaults from zqk.yaml, environment overrides from zqk-{env}.yaml, and local overrides from zqk-local.yaml.
func LoadForRoot(root string) *ZqkConfig {
	if root == "" {
		return &ZqkConfig{}
	}
	cfg, _ := rootConfigs.Load(root, configStamp(root), func() (*ZqkConfig, error) {
		return readConfigForRoot(root), nil
	})
	if cfg == nil {
		return &ZqkConfig{}
	}
	return cfg
}

func configStamp(root string) stampmemo.Stamp {
	envName := strings.TrimSpace(zqkenv.ZqkEnv().Get())
	if envName == "" {
		envName = strings.TrimSpace(os.Getenv("ENVIRONMENT"))
	}
	return stampmemo.OfAll(
		filepath.Join(root, paths.ConfigDir, paths.ZqkConfigFileName),
		filepath.Join(root, paths.ZqkConfigFileName),
		filepath.Join(root, paths.ConfigDir, paths.ZqkEnvConfigFilePrefix+envName+paths.YAMLExtension),
		filepath.Join(root, paths.ZqkEnvConfigFilePrefix+envName+paths.YAMLExtension),
		filepath.Join(root, paths.ConfigDir, paths.ZqkLocalConfigFileName),
		filepath.Join(root, paths.ZqkLocalConfigFileName),
	)
}

func readConfigForRoot(root string) *ZqkConfig {
	cfg := &ZqkConfig{}

	// 1. Load committed defaults: config/zqk.yaml (fallback: zqk.yaml at root)
	defaultPath := filepath.Join(root, paths.ConfigDir, paths.ZqkConfigFileName)
	if stampmemo.Of(defaultPath) == 0 {
		rootDefault := filepath.Join(root, paths.ZqkConfigFileName)
		if stampmemo.Of(rootDefault) != 0 {
			defaultPath = rootDefault
		}
	}
	if data, err := fileutil.ReadFile(defaultPath); err == nil {
		_ = yaml.Unmarshal(data, cfg)
	}

	// 2. Load environment-specific overrides: config/zqk-{env}.yaml
	envName := strings.TrimSpace(zqkenv.ZqkEnv().Get())
	if envName == "" {
		envName = strings.TrimSpace(os.Getenv("ENVIRONMENT"))
	}
	if envName != "" && envName != "local" {
		envPath := filepath.Join(root, paths.ConfigDir, paths.ZqkEnvConfigFilePrefix+envName+paths.YAMLExtension)
		if stampmemo.Of(envPath) == 0 {
			rootEnv := filepath.Join(root, paths.ZqkEnvConfigFilePrefix+envName+paths.YAMLExtension)
			if stampmemo.Of(rootEnv) != 0 {
				envPath = rootEnv
			}
		}
		if data, err := fileutil.ReadFile(envPath); err == nil {
			_ = yaml.Unmarshal(data, cfg)
		}
	}

	// 3. Load local overrides: config/zqk-local.yaml (fallback: zqk-local.yaml at root)
	localPath := filepath.Join(root, paths.ConfigDir, paths.ZqkLocalConfigFileName)
	if stampmemo.Of(localPath) == 0 {
		rootLocal := filepath.Join(root, paths.ZqkLocalConfigFileName)
		if stampmemo.Of(rootLocal) != 0 {
			localPath = rootLocal
		}
	}
	if data, err := fileutil.ReadFile(localPath); err == nil {
		_ = yaml.Unmarshal(data, cfg)
	}

	return cfg
}
