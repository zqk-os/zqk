package quality

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestBuildMatrixActivityLogEntry_shape(t *testing.T) {
	t.Parallel()
	e := BuildMatrixActivityLogEntry("codebase_vetting", "/repo/docs/x.csv", false, 0, map[string]string{"a": "1", "b": "2"})
	if e["action"] != "matrix_update" {
		t.Fatalf("action: %#v", e["action"])
	}
	if e[objects.FieldKeyPhase] != "" {
		t.Fatalf("phase: %#v", e[objects.FieldKeyPhase])
	}
	ts, _ := e["timestamp"].(string)
	if ts == "" {
		t.Fatal("expected timestamp")
	}
	notes, _ := e[objects.FieldKeyNotes].(string)
	if notes == "" {
		t.Fatal("expected notes")
	}
}
