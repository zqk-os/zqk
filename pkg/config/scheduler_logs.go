package config

import (
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// SchedulerLogsConfig is the root config for scheduler log retention (per-job logs).
// Aligns with .zqk/specs/configs/scheduler_logs_config.yaml.
type SchedulerLogsConfig struct {
	JobLogs JobLogsConfig `yaml:"job_logs,omitempty"`
}

// JobLogsConfig configures per-job log file retention (.zqk/logs/scheduler/<job-id>/<job-id>.log).
type JobLogsConfig struct {
	// MaxLines is the maximum number of JSONL lines kept per job log file (rolling retention).
	// 0 = no trim (unbounded). Default 500 when config file is missing.
	MaxLines int `yaml:"max_lines,omitempty"`
}

const defaultMaxJobLogLines = 500

var schedulerLogsYAML stampmemo.Table[*SchedulerLogsConfig]

// SchedulerLogsLoader loads scheduler log config from YAML.
type SchedulerLogsLoader struct {
	configPath string
}

// NewSchedulerLogsLoader creates a loader. configPath is optional; if empty, path is resolved from projectRoot.
func NewSchedulerLogsLoader(projectRoot string) *SchedulerLogsLoader {
	path := LoggingSchedulerLogsConfig().Safe()
	if path == emptyPath && projectRoot != emptyPath {
		path = filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, "scheduler_logs_config.yaml")
	}
	return &SchedulerLogsLoader{configPath: path}
}

func defaultSchedulerLogsConfig() *SchedulerLogsConfig {
	return &SchedulerLogsConfig{JobLogs: JobLogsConfig{MaxLines: defaultMaxJobLogLines}}
}

// Load loads and returns the scheduler logs config. Missing file returns default (max_lines 500) and nil error.
func (l *SchedulerLogsLoader) Load() (*SchedulerLogsConfig, error) {
	return schedulerLogsYAML.Load(l.configPath, stampmemo.Of(l.configPath), func() (*SchedulerLogsConfig, error) {
		if l.configPath == emptyPath {
			return defaultSchedulerLogsConfig(), nil
		}
		data, readErr := fileutil.ReadFile(l.configPath)
		if readErr != nil {
			if fileutil.IsNotExist(readErr) {
				return defaultSchedulerLogsConfig(), nil
			}
			return nil, errfmt.Newf("read scheduler logs config").Wrap(readErr)
		}
		var parsed SchedulerLogsConfig
		if parseErr := yaml.Unmarshal(data, &parsed); parseErr != nil {
			return nil, errfmt.Newf("parse scheduler logs config").Wrap(parseErr)
		}
		if parsed.JobLogs.MaxLines <= 0 {
			parsed.JobLogs.MaxLines = defaultMaxJobLogLines
		}
		return &parsed, nil
	})
}

// GetMaxJobLogLines returns the configured max lines per job log (rolling retention).
// Uses projectRoot to resolve config path; returns default (500) when config is missing or invalid.
func GetMaxJobLogLines(projectRoot string) int {
	loader := NewSchedulerLogsLoader(projectRoot)
	c, err := loader.Load()
	if err != nil || c == nil {
		return defaultMaxJobLogLines
	}
	if c.JobLogs.MaxLines <= 0 {
		return defaultMaxJobLogLines
	}
	return c.JobLogs.MaxLines
}
