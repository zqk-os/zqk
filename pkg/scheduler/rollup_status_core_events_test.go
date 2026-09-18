package scheduler

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestAppendRollupStatusCoreEvent_WritesJSONL(t *testing.T) {
	root := t.TempDir()
	rollup := map[string]any{
		"rollup_status":          "satisfied",
		objects.FieldKeyBlockers: []any{},
	}
	AppendRollupStatusCoreEvent(root, "CVS-test", rollup)
	p := RollupStatusCoreEventsFilePath(root)
	b, err := fileutil.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var line map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(b))), &line); err != nil {
		t.Fatalf("json: %v\n%s", err, b)
	}
	if line["convergence_session_id"] != "CVS-test" {
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
