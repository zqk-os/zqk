package datacell

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metricsrecording"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestAppendStewardEnqueueRecord_appendsMetricsJSONLWhenRecordingEnabled(t *testing.T) {
	t.Parallel()
	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	root := t.TempDir()
	log := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	rec := BuildStewardEnqueueRecord(ProfileStream, MaintenanceOp{Name: MaintenanceOpRefreshSummary, Detail: "probe"})
	if err := AppendStewardEnqueueRecord(root, rec); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, paths.ProjectDataDir, paths.MetricsDir, stewardMetricsJSONLFile)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(b))
	var row map[string]any
	if err := json.Unmarshal([]byte(line), &row); err != nil {
		t.Fatal(err)
	}
	if row["event"] != "steward_enqueue" {
		t.Fatalf("event: %v", row["event"])
	}
	if row["op"] != MaintenanceOpRefreshSummary {
		t.Fatalf("op: %v", row["op"])
	}
	_ = log
}

func TestDrainStewardEnqueueLog_metricsDrainRow(t *testing.T) {
	t.Parallel()
	metricsrecording.EnterAllowRecording()
	t.Cleanup(metricsrecording.LeaveAllowRecording)

	root := t.TempDir()
	log := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	if err := AppendStewardEnqueueRecord(root, BuildStewardEnqueueRecord(ProfileStream, MaintenanceOp{Name: MaintenanceOpInvalidateCache})); err != nil {
		t.Fatal(err)
	}
	const tickJob = "SCH-dce-tick-test"
	n, err := DrainStewardEnqueueLog(root, log, tickJob)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("drained=%d", n)
	}
	p := filepath.Join(root, paths.ProjectDataDir, paths.MetricsDir, stewardMetricsJSONLFile)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) < 2 {
		t.Fatalf("want enqueue + drain metrics lines, got %d", len(lines))
	}
	var drainRow map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &drainRow); err != nil {
		t.Fatal(err)
	}
	if drainRow["event"] != "steward_drain" {
		t.Fatalf("last event: %v", drainRow["event"])
	}
	if drainRow["data_cell_envelope_tick_job_id"] != tickJob {
		t.Fatalf("tick job id: %v", drainRow["data_cell_envelope_tick_job_id"])
	}
}
