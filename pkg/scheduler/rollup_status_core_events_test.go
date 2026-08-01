package scheduler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestAppendRollupStatusCoreEvent_WritesJSONL(t *testing.T) {
	root := t.TempDir()
	rollup := map[string]any{
		"rollup_status":          "satisfied",
		objects.FieldKeyBlockers: []any{},
	}
	AppendRollupStatusCoreEvent(root, "CONV-test", rollup)
	p := RollupStatusCoreEventsFilePath(root)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var line map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(b))), &line); err != nil {
		t.Fatalf("json: %v\n%s", err, b)
	}
	if line["convergence_session_id"] != "CONV-test" {
		t.Fatalf("session id: %v", line["convergence_session_id"])
	}
	rsc, ok := line["rollup_status_core"].(map[string]any)
	if !ok || rsc["rollup_status"] != "satisfied" {
		t.Fatalf("rollup: %#v", line["rollup_status_core"])
	}
	if filepath.Base(p) != "rollup_status_core.jsonl" {
		t.Fatalf("path: %s", p)
	}
}
