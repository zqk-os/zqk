package scheduler

import (
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
)

// loadSchedulerConfig loads scheduler configuration (delegates to pkg/scheduler for shared use with daemon).
func loadSchedulerConfig(projectRoot string) (*schedulerConfig, error) {
	cfg, err := schedulerpkg.LoadSchedulerConfig(projectRoot)
	if err != nil {
		return nil, err
	}
	return &schedulerConfig{
		Enabled:     cfg.Enabled,
		ProjectType: cfg.ProjectType,
		JobsPaused:  cfg.JobsPaused,
	}, nil
}

// schedulerConfig is the cmd-layer view of config (same shape as schedulerpkg.SchedulerConfig).
type schedulerConfig struct {
	Enabled     bool
	ProjectType string
	JobsPaused  bool
}

// saveSchedulerConfig writes the scheduler config (delegates to pkg/scheduler).
func saveSchedulerConfig(projectRoot string, config *schedulerConfig) error {
	return schedulerpkg.SaveSchedulerConfig(projectRoot, &schedulerpkg.SchedulerConfig{
		Enabled:     config.Enabled,
		ProjectType: config.ProjectType,
		JobsPaused:  config.JobsPaused,
	})
}
