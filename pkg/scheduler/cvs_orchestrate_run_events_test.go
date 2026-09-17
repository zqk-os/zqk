package scheduler

import (
	"encoding/json"
	"strings"
	"testing"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestAppendCVSOrchestrateRunV1_WritesJSONL(t *testing.T) {
	root := t.TempDir()
	rec := &CVSOrchestrateRunV1{
		SchemaVersion:  CVSOrchestrateRunSchemaVersion,
		EndedAtMS:      100,
		DurationMS:     50,
		CvsID:          "CVS-test",
		ExitCode:       0,
		Stage:          "rollup",
		PersistSkipped: false,
		PersistFailed:  false,
		RollupStatus:   "satisfied",
	}
	if err := AppendCVSOrchestrateRunV1(root, "", rec); err != nil {
		t.Fatal(err)
	}
	p := CVSOrchestrateRunsFilePath(root)
	b, err := fileutil.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var line CVSOrchestrateRunV1
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(b))), &line); err != nil {
		t.Fatalf("json: %v\n%s", err, b)
	}
	if line.CvsID != "CVS-test" || line.RollupStatus != "satisfied" {
		t.Fatalf("record: %+v", line)
	}
}

func TestAppendCVSOrchestrateRunV1_RejectsBadSchema(t *testing.T) {
	root := t.TempDir()
	rec := &CVSOrchestrateRunV1{SchemaVersion: "wrong"}
	err := AppendCVSOrchestrateRunV1(root, "", rec)
	if err == nil {
		t.Fatal("want error")
	}
}
