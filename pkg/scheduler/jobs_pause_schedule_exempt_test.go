package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestLoadJobsPausedScheduleExemptIDs_FromYAML(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfgDir := filepath.Join(root, paths.ProcessInternalConfigsDir)
	if err := fileutil.EnsureDir(cfgDir); err != nil {
		t.Fatal(err)
	}
	yaml := `jobs_paused_schedule_exempt_job_ids:
  - SCH-exempt-from-yaml
required_jobs: []
`
	cfgPath := filepath.Join(cfgDir, "scheduler_maintenance_config.yaml")
	if err := fileutil.WriteSecureFile(cfgPath, []byte(yaml)); err != nil {
		t.Fatal(err)
	}

	m := loadJobsPausedScheduleExemptIDs(root)
	if m == nil || len(m) != 1 || !m["SCH-exempt-from-yaml"] {
		t.Fatalf("expected map SCH-exempt-from-yaml, got %#v", m)
	}
}

func TestScheduler_JobsPausedScheduleExempt_WiresFromMaintenanceYAML(t *testing.T) {
	restoreGlobal := WithGlobalSchedulerRollback()
	t.Cleanup(restoreGlobal)
	testRoot, storage := prepareHandlersIsolatedTempProject(t)
	cfgDir := filepath.Join(testRoot, paths.ProcessInternalConfigsDir)
	if err := fileutil.EnsureDir(cfgDir); err != nil {
		t.Fatal(err)
	}
	yaml := `jobs_paused_schedule_exempt_job_ids:
  - SCH-exempt-from-yaml
required_jobs: []
`
	if err := fileutil.WriteSecureFile(filepath.Join(cfgDir, "scheduler_maintenance_config.yaml"), []byte(yaml)); err != nil {
		t.Fatal(err)
	}

	specLoader := objects.NewSpecLoader(testRoot)
	lifecycleLoader := objects.NewLifecycleLoader(testRoot)
	sched := NewSchedulerWithProjectRoot(storage, specLoader, lifecycleLoader, testRoot, nil).(*Scheduler)
	if sched.jobsPausedScheduleExemptIDs == nil || !sched.jobsPausedScheduleExemptIDs["SCH-exempt-from-yaml"] {
		t.Fatal("expected scheduler to load exemption ids from maintenance YAML")
	}
}
