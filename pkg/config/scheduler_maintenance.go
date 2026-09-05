package config

import (
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

// SchedulerMaintenanceConfig is the root config for required scheduler maintenance jobs.
// Aligns with docs/process/_internal/configs/scheduler_maintenance_config.yaml.
// Single source of truth: all ensure-maintenance flows read this so behavior is
// observable, reliable, accurate, and efficient.
type SchedulerMaintenanceConfig struct {
	// JobsPausedScheduleExemptJobIDs lists scheduler_job ids that keep timer/immediate scheduling when
	// jobs_paused is enabled. Loaded by the scheduler from this YAML (override via ZQK_SCHEDULER_MAINTENANCE_CONFIG).
	JobsPausedScheduleExemptJobIDs []string           `yaml:"jobs_paused_schedule_exempt_job_ids,omitempty"`
	RequiredJobs                   []RequiredJobEntry `yaml:"required_jobs,omitempty"`
}

// RequiredJobEntry defines one scheduler_job that must exist.
type RequiredJobEntry struct {
	ID           string `yaml:"id"`
	JobType      string `yaml:"job_type"`
	TemplateFile string `yaml:"template_file"`
}

// SchedulerMaintenanceLoader loads scheduler maintenance config from YAML.
type SchedulerMaintenanceLoader struct {
	configPath string
	mu         sync.Mutex
	cache      *SchedulerMaintenanceConfig
}

// NewSchedulerMaintenanceLoader creates a loader. configPath is optional; if empty, path is resolved from projectRoot.
func NewSchedulerMaintenanceLoader(projectRoot string) *SchedulerMaintenanceLoader {
	path := os.Getenv(zqkenv.SchedulerMaintenanceConfig())
	if path == emptyPath && projectRoot != emptyPath {
		path = filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, "scheduler_maintenance_config.yaml")
	}
	return &SchedulerMaintenanceLoader{configPath: path}
}

// Load loads and returns the scheduler maintenance config. Missing file returns nil, error.
func (l *SchedulerMaintenanceLoader) Load() (*SchedulerMaintenanceConfig, error) {
	var c *SchedulerMaintenanceConfig
	err := concurrency.RunInLock(&l.mu, func() error {
		if l.configPath == emptyPath {
			return errfmt.Errorf("scheduler maintenance config path is empty")
		}
		data, readErr := fileutil.ReadFile(l.configPath)
		if readErr != nil {
			if fileutil.IsNotExist(readErr) {
				return errfmt.Errorf("scheduler maintenance config not found: %s", l.configPath)
			}
			return errfmt.Newf("read scheduler maintenance config").Wrap(readErr)
		}
		var parsed SchedulerMaintenanceConfig
		if parseErr := yaml.Unmarshal(data, &parsed); parseErr != nil {
			return errfmt.Newf("parse scheduler maintenance config").Wrap(parseErr)
		}
		l.cache = &parsed
		c = &parsed
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}
