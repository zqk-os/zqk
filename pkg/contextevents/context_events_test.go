package contextevents

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestAppend_WritesJSONLLine(t *testing.T) {
	root := t.TempDir()
	rec := &Record{
		EventType:             "matrix_verify",
		Source:                "test",
		CorrelationID:         "corr-1",
		ConvergenceSessionRef: "CONV-demo",
		CriteriaRefs:          []string{" CRIT-a ", "CRIT-b"},
		BacklogItemRefs:       []string{"ITEM-x"},
		JobID:                 "SCH-demo",
		Note:                  "note",
		Payload:               map[string]any{objects.FieldKeyTitle: "test_bundle", objects.FieldKeyStatus: "ok"},
	}
	if err := Append(root, rec); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, paths.ProjectDataDir, paths.MetricsDir, paths.ContextEventsJSONLFile)
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	line := bytes.TrimSpace(data)
	var got Record
	if err := json.Unmarshal(line, &got); err != nil {
		t.Fatal(err)
	}
	if got.EventType != "matrix_verify" {
		t.Fatalf("%s: %q", objects.FieldKeyEventType, got.EventType)
	}
	if got.SchemaVersion != SchemaVersionV1 {
		t.Fatalf("%s: %q", objects.FieldKeySchemaVersion, got.SchemaVersion)
	}
	if got.TsRFC3339 == "" {
		t.Fatalf("missing %s", WireTsRFC3339)
	}
	if len(got.CriteriaRefs) != 2 || got.CriteriaRefs[0] != "CRIT-a" {
		t.Fatalf("%s: %#v", objects.FieldKeyCriteriaRefs, got.CriteriaRefs)
	}
	if got.Payload[objects.FieldKeyTitle] != "test_bundle" {
		t.Fatalf("%s: %#v", objects.FieldKeyPayload, got.Payload)
	}
}

func TestAppend_EmptyEventTypeErrors(t *testing.T) {
	root := t.TempDir()
	err := Append(root, &Record{EventType: "  "})
	if err != ErrEmptyEventType {
		t.Fatalf("got %v want ErrEmptyEventType", err)
	}
}

func TestMergePayloadJSON(t *testing.T) {
	dst := map[string]any{objects.FieldKeyTitle: 1}
	inner := map[string]any{objects.FieldKeyDescription: float64(2)}
	raw, err := json.Marshal(inner)
	if err != nil {
		t.Fatal(err)
	}
	out, err := MergePayloadJSON(dst, raw)
	if err != nil {
		t.Fatal(err)
	}
	if out[objects.FieldKeyTitle] != 1 || out[objects.FieldKeyDescription] != float64(2) {
		t.Fatalf("%#v", out)
	}
}

func TestUnmarshalJSON_LegacyKeys(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		WireTsRFC3339:                  "2026-01-01T00:00:00Z",
		objects.FieldKeyEventType:      "x",
		objects.FieldKeySchemaVersion:  "1",
		wireLegacyBacklogItemIDs:       []string{"ITEM-1"},
		wireLegacyConvergenceSessionID: "CONV-old",
	})
	if err != nil {
		t.Fatal(err)
	}
	var r Record
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	if len(r.BacklogItemRefs) != 1 || r.BacklogItemRefs[0] != "ITEM-1" {
		t.Fatalf("legacy backlog: %#v", r.BacklogItemRefs)
	}
	if r.ConvergenceSessionRef != "CONV-old" {
		t.Fatalf("legacy cvs: %q", r.ConvergenceSessionRef)
	}
}
