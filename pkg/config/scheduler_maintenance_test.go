package config

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// findModuleRoot walks up from the test working directory to the Go module root (go.mod).
func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found from test wd")
		}
		dir = parent
	}
}

// TestSchedulerMaintenanceConfig_IncludesDataCellEnvelopeTick locks in the v1 operational-envelope
// timer job entry in repo config: ensure-retention-jobs and daemon startup
// rely on docs/process/_internal/configs/scheduler_maintenance_config.yaml.
func TestSchedulerMaintenanceConfig_IncludesDataCellEnvelopeTick(t *testing.T) {
	t.Parallel()
	root := findModuleRoot(t)
	loader := NewSchedulerMaintenanceLoader(root)
	cfg, err := loader.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	const (
		wantID      = "SCH-dce-tick"
		wantJobType = "data_cell_envelope_tick"
		wantTmpl    = "scripts/scheduler_jobs/data_cell_envelope_tick_six_hourly.yaml"
	)
	var found bool
	for _, e := range cfg.RequiredJobs {
		if e.ID != wantID {
			continue
		}
		found = true
		if e.JobType != wantJobType {
			t.Errorf("required_jobs entry %s: job_type = %q, want %q", wantID, e.JobType, wantJobType)
		}
		if e.TemplateFile != wantTmpl {
			t.Errorf("required_jobs entry %s: template_file = %q, want %q", wantID, e.TemplateFile, wantTmpl)
		}
		break
	}
	if !found {
		t.Fatalf("required_jobs missing entry id=%q (data cell envelope tick)", wantID)
	}
	if len(cfg.JobsPausedScheduleExemptJobIDs) == 0 {
		t.Fatal("jobs_paused_schedule_exempt_job_ids must be non-empty (scheduler jobs_paused exemption)")
	}
	// Keep in sync with jobs_paused_schedule_exempt_job_ids in
	// docs/process/_internal/configs/scheduler_maintenance_config.yaml
	wantExempt := map[string]bool{
		"SCH-evag":                    true,
		"SCH-cache-prewarm":           true,
		"SCH-retention-tolerance":     true,
		"SCH-audit-event-aggregation": true,
		"SCH-cap-orchestrator":        true,
		"SCH-autofix-process-pending": true,
	}
	gotExempt := make(map[string]bool, len(cfg.JobsPausedScheduleExemptJobIDs))
	for _, id := range cfg.JobsPausedScheduleExemptJobIDs {
		gotExempt[id] = true
	}
	for id := range wantExempt {
		if !gotExempt[id] {
			t.Errorf("jobs_paused_schedule_exempt_job_ids missing %q", id)
		}
	}
	tmplPath := filepath.Join(root, wantTmpl)
	if st, err := fileutil.Stat(tmplPath); err != nil || st.IsDir() {
		t.Fatalf("template file missing or not a file: %s (%v)", tmplPath, err)
	}
	cfgPath := filepath.Join(root, paths.ProcessInternalConfigsDir, "scheduler_maintenance_config.yaml")
	if st, err := fileutil.Stat(cfgPath); err != nil || st.IsDir() {
		t.Fatalf("maintenance config missing: %s (%v)", cfgPath, err)
	}
}
