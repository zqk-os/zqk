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

// SchedulerLogsConfig is the root config for scheduler log retention (per-job logs).
// Aligns with docs/process/_internal/configs/scheduler_logs_config.yaml.
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

// SchedulerLogsLoader loads scheduler log config from YAML.
type SchedulerLogsLoader struct {
	configPath string
	mu         sync.Mutex
	cache      *SchedulerLogsConfig
}

// NewSchedulerLogsLoader creates a loader. configPath is optional; if empty, path is resolved from projectRoot.
func NewSchedulerLogsLoader(projectRoot string) *SchedulerLogsLoader {
	path := os.Getenv(zqkenv.SchedulerLogsConfig())
	if path == emptyPath && projectRoot != emptyPath {
		path = filepath.Join(projectRoot, paths.ProcessInternalConfigsDir, "scheduler_logs_config.yaml")
	}
	return &SchedulerLogsLoader{configPath: path}
}

// Load loads and returns the scheduler logs config. Missing file returns default (max_lines 500) and nil error.
func (l *SchedulerLogsLoader) Load() (*SchedulerLogsConfig, error) {
	var c *SchedulerLogsConfig
	err := concurrency.RunInLock(&l.mu, func() error {
		if l.configPath == emptyPath {
			c = &SchedulerLogsConfig{JobLogs: JobLogsConfig{MaxLines: defaultMaxJobLogLines}}
			return nil
		}
		data, readErr := fileutil.ReadFile(l.configPath)
		if readErr != nil {
			if fileutil.IsNotExist(readErr) {
				c = &SchedulerLogsConfig{JobLogs: JobLogsConfig{MaxLines: defaultMaxJobLogLines}}
				return nil
			}
			return errfmt.Newf("read scheduler logs config").Wrap(readErr)
		}
		var parsed SchedulerLogsConfig
		if parseErr := yaml.Unmarshal(data, &parsed); parseErr != nil {
			return errfmt.Newf("parse scheduler logs config").Wrap(parseErr)
		}
		if parsed.JobLogs.MaxLines <= 0 {
			parsed.JobLogs.MaxLines = defaultMaxJobLogLines
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
