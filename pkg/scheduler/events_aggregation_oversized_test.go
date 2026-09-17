package scheduler

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestAggregateEventsFromFile_SkipsOversizedNonJobLines(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	eventsPath := filepath.Join(dir, "diagnostics.jsonl")

	// ~1MB MCP-style dump (exceeds former 512KiB Scanner cap) followed by a real job event.
	huge := `{"error":"` + strings.Repeat("x", 1024*1024) + `","level":"error","message":"MCP Tool execution returned error","toolName":"zqk_object_list","timestamp":"2026-08-12T00:00:00Z"}`
	job := `{"event_type":"scheduler_job_completed","job_id":"SCH-test","duration_seconds":1.5,"timestamp":"2026-08-12T00:00:01Z"}`
	if err := fileutil.WriteSecureFile(eventsPath, []byte(huge+"\n"+job+"\n")); err != nil {
		t.Fatal(err)
	}

	summary, linesRead, err := AggregateEventsFromFile(eventsPath)
	if err != nil {
		t.Fatalf("AggregateEventsFromFile: %v", err)
	}
	if linesRead != 2 {
		t.Fatalf("linesRead=%d, want 2", linesRead)
	}
	st, ok := summary.JobStats["SCH-test"]
	if !ok || st.Completed != 1 {
		t.Fatalf("job stats=%v, want SCH-test completed=1", summary.JobStats)
	}
	if _, err := fileutil.Stat(eventsPath); err != nil {
		t.Fatal(err)
	}
}
