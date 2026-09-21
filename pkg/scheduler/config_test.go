package scheduler

import (
	"testing"
)

func TestLoadSchedulerConfig_reloadsAfterSave(t *testing.T) {
	root := t.TempDir()
	cfg, err := LoadSchedulerConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.JobsPaused {
		t.Fatal("default JobsPaused must be false")
	}
	cfg.JobsPaused = true
	if err := SaveSchedulerConfig(root, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSchedulerConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if !got.JobsPaused {
		t.Fatal("Load after Save must see JobsPaused")
	}
}
