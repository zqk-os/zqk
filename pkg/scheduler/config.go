package scheduler

import (
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// ReloadConfigRequestFile is the name of the file the CLI writes to signal the daemon to reload config.
// When present in .zqk/scheduler/, the daemon reloads config and then removes the file.
const ReloadConfigRequestFile = "reload_config_request"

// SchedulerConfig holds scheduler configuration (persisted at .zqk/scheduler/config.yaml).
// Used by both the daemon (to read jobs_paused) and the CLI (to get/set config).
type SchedulerConfig struct {
	Enabled     bool   `yaml:"enabled"`
	ProjectType string `yaml:"project_type"`
	// JobsPaused when true: scheduler runs but no timer or immediate jobs run; only manually triggered jobs run.
	JobsPaused bool `yaml:"jobs_paused"`
}

var schedulerConfigs stampmemo.Table[*SchedulerConfig] // keyed by projectRoot; stamp is config.yaml

// LoadSchedulerConfig loads scheduler configuration from the scheduler config file.
// Returns default (enabled, jobs not paused) if the file does not exist.
func LoadSchedulerConfig(projectRoot string) (*SchedulerConfig, error) {
	schedulerRoot := paths.ResolvePathFromCacheOrConstant(projectRoot, "scheduler", filepath.Join(paths.ProjectDataDir, paths.SchedulerDir))
	configPath := filepath.Join(schedulerRoot, paths.SchedulerConfigFile)
	return schedulerConfigs.Load(projectRoot, stampmemo.Of(configPath), func() (*SchedulerConfig, error) {
		return readSchedulerConfig(configPath)
	})
}

func readSchedulerConfig(configPath string) (*SchedulerConfig, error) {
	data, err := fileutil.ReadFile(configPath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return &SchedulerConfig{
				Enabled:     true,
				ProjectType: "production",
				JobsPaused:  false,
			}, nil
		}
		return nil, errfmt.Newf("failed to read scheduler config").Wrap(err)
	}

	var config SchedulerConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, errfmt.Newf("failed to parse scheduler config").Wrap(err)
	}

	if config.ProjectType == emptyValue {
		config.ProjectType = "production"
	}

	return &config, nil
}

// SaveSchedulerConfig writes the scheduler config to the project's .zqk/scheduler/config.yaml.
func SaveSchedulerConfig(projectRoot string, config *SchedulerConfig) error {
	dir := paths.ResolvePathFromCacheOrConstant(projectRoot, "scheduler", filepath.Join(paths.ProjectDataDir, paths.SchedulerDir))
	if err := fileutil.EnsureDir(dir); err != nil {
		return errfmt.Newf("failed to create scheduler config dir").Wrap(err)
	}
	configPath := filepath.Join(dir, paths.SchedulerConfigFile)
	data, err := yaml.Marshal(config)
	if err != nil {
		return errfmt.Newf("failed to marshal scheduler config").Wrap(err)
	}
	if err := fileutil.WriteSecureFile(configPath, data); err != nil {
		return errfmt.Newf("failed to write scheduler config").Wrap(err)
	}
	schedulerConfigs.Delete(projectRoot)
	return nil
}

// WriteReloadConfigRequest creates the reload request file so the running daemon picks up config changes immediately.
// Call this after SaveSchedulerConfig when the CLI has updated config (e.g. --no-jobs-paused).
func WriteReloadConfigRequest(projectRoot string) error {
	dir := paths.ResolvePathFromCacheOrConstant(projectRoot, "scheduler", filepath.Join(paths.ProjectDataDir, paths.SchedulerDir))
	if err := fileutil.EnsureDir(dir); err != nil {
		return errfmt.Newf("failed to ensure scheduler dir for reload request").Wrap(err)
	}
	path := filepath.Join(dir, ReloadConfigRequestFile)
	return fileutil.WriteSecureFile(path, nil)
}

// ConsumeReloadConfigRequest removes the reload request file if present and returns true if it existed.
// The daemon calls this to detect that the CLI requested an immediate config reload.
func ConsumeReloadConfigRequest(projectRoot string) bool {
	path := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, ReloadConfigRequestFile)
	if err := fileutil.Remove(path); err != nil {
		return false
	}
	return true
}

// ReloadConfigRequestExists returns true if the reload request file exists (daemon should reload config).
func ReloadConfigRequestExists(projectRoot string) bool {
	dir := paths.ResolvePathFromCacheOrConstant(projectRoot, "scheduler", filepath.Join(paths.ProjectDataDir, paths.SchedulerDir))
	path := filepath.Join(dir, ReloadConfigRequestFile)
	if info, err := fileutil.Stat(path); err != nil {
		_ = info // Acknowledged
		return false
	}
	return true
}
