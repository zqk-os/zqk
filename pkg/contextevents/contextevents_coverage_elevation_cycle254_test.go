package contextevents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

func TestContextEventsJSONLPath(t *testing.T) {
	if got := ContextEventsJSONLPath(""); got != "" {
		t.Fatalf("expected empty string for empty projectRoot, got %q", got)
	}

	root := "/path/to/project"
	expected := filepath.Join(root, paths.ProjectDataDir, paths.MetricsDir, paths.ContextEventsJSONLFile)
	if got := ContextEventsJSONLPath(root); got != expected {
		t.Fatalf("expected %q, got %q", expected, got)
	}
}

func TestAppend_EdgeCases(t *testing.T) {
	// 1. Empty projectRoot returns nil
	if err := Append("", &Record{EventType: "ev_test"}); err != nil {
		t.Fatalf("expected nil error on empty root, got %v", err)
	}

	// 2. Nil record returns nil
	root := t.TempDir()
	if err := Append(root, nil); err != nil {
		t.Fatalf("expected nil error on nil record, got %v", err)
	}

	// 3. Empty or whitespace event type
	if err := Append(root, &Record{EventType: ""}); err != ErrEmptyEventType {
		t.Fatalf("expected ErrEmptyEventType, got %v", err)
	}
	if err := Append(root, &Record{EventType: "   \t\n"}); err != ErrEmptyEventType {
		t.Fatalf("expected ErrEmptyEventType for whitespace, got %v", err)
	}

	// 4. Custom schema version and explicit timestamp preserved
	customTS := "2026-10-30T15:00:00Z"
	customSchema := "v2-custom"
	recExplicit := &Record{
		EventType:     "custom_event",
		SchemaVersion: customSchema,
		TsRFC3339:     customTS,
		Note:          "short note",
	}
	if err := Append(root, recExplicit); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// 5. Truncate note exceeding noteMaxRunes (4096 runes)
	longRunes := make([]rune, 4100)
	for i := range longRunes {
		longRunes[i] = 'Z'
	}
	longNote := string(longRunes)
	recLong := &Record{
		EventType: "long_note_event",
		Note:      longNote,
	}
	if err := Append(root, recLong); err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	if len([]rune(recLong.Note)) != noteMaxRunes {
		t.Fatalf("expected note to be truncated to %d runes, got %d", noteMaxRunes, len([]rune(recLong.Note)))
	}

	// Note with Unicode multi-byte runes
	unicodeRunes := make([]rune, 4105)
	for i := range unicodeRunes {
		unicodeRunes[i] = '★'
	}
	recUnicode := &Record{
		EventType: "unicode_event",
		Note:      string(unicodeRunes),
	}
	if err := Append(root, recUnicode); err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	if len([]rune(recUnicode.Note)) != noteMaxRunes {
		t.Fatalf("expected unicode note to be truncated to %d runes, got %d", noteMaxRunes, len([]rune(recUnicode.Note)))
	}

	// 6. Normalization of slice fields: whitespace-only elements trimmed and filtered
	recSlices := &Record{
		EventType:       "slice_event",
		CriteriaRefs:    []string{"  crit-1  ", "", "  ", "crit-2"},
		BacklogItemRefs: []string{"   ", "\t"},
	}
	if err := Append(root, recSlices); err != nil {
		t.Fatalf("Append failed: %v", err)
	}
	if len(recSlices.CriteriaRefs) != 2 || recSlices.CriteriaRefs[0] != "crit-1" || recSlices.CriteriaRefs[1] != "crit-2" {
		t.Fatalf("unexpected CriteriaRefs: %#v", recSlices.CriteriaRefs)
	}
	if recSlices.BacklogItemRefs != nil {
		t.Fatalf("expected BacklogItemRefs to be nil after trimming whitespace, got %#v", recSlices.BacklogItemRefs)
	}
}

func TestMergePayloadJSON_EdgeCases(t *testing.T) {
	dst := map[string]any{"initial": "val"}

	// 1. Zero-length raw payload
	out, err := MergePayloadJSON(dst, nil)
	if err != nil || out["initial"] != "val" {
		t.Fatalf("unexpected result for nil raw: out=%v, err=%v", out, err)
	}
	out, err = MergePayloadJSON(dst, []byte{})
	if err != nil || out["initial"] != "val" {
		t.Fatalf("unexpected result for empty raw: out=%v, err=%v", out, err)
	}

	// 2. Whitespace-only raw payload
	out, err = MergePayloadJSON(dst, []byte("   \n\t  "))
	if err != nil || out["initial"] != "val" {
		t.Fatalf("unexpected result for whitespace raw: out=%v, err=%v", out, err)
	}

	// 3. Exceeds payloadJSONMaxBytes (65536)
	oversized := make([]byte, payloadJSONMaxBytes+1)
	for i := range oversized {
		oversized[i] = ' '
	}
	out, err = MergePayloadJSON(dst, oversized)
	if err != ErrPayloadTooLarge {
		t.Fatalf("expected ErrPayloadTooLarge, got err=%v", err)
	}

	// 4. Malformed JSON
	malformed := []byte(`{not valid json}`)
	out, err = MergePayloadJSON(dst, malformed)
	if err == nil {
		t.Fatalf("expected error for malformed JSON, got nil")
	}

	// 5. dst is nil, allocates new map
	validJSON := []byte(`{"key1": "value1", "key2": 42}`)
	out, err = MergePayloadJSON(nil, validJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["key1"] != "value1" || out["key2"] != float64(42) {
		t.Fatalf("unexpected merged map: %#v", out)
	}

	// 6. Overwrites prior keys
	overwriteJSON := []byte(`{"initial": "updated", "extra": true}`)
	out, err = MergePayloadJSON(dst, overwriteJSON)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["initial"] != "updated" || out["extra"] != true {
		t.Fatalf("expected key overwrite, got %#v", out)
	}
}

func TestRecord_MarshalJSON_FullAndEmpty(t *testing.T) {
	// Full record
	full := &Record{
		TsRFC3339:             "2026-10-30T10:00:00Z",
		SchemaVersion:         SchemaVersionV1,
		EventType:             "system_signal",
		Source:                DefaultSourceCLI,
		CorrelationID:         "corr-alpha-100",
		JobID:                 "job-zeta-200",
		Note:                  "human verification complete",
		CriteriaRefs:          []string{"crit-one", "crit-two"},
		BacklogItemRefs:       []string{"bli-one"},
		ConvergenceSessionRef: "cvs-stream-01",
		Payload:               map[string]any{"status": "ok", "retries": float64(0)},
	}

	data, err := json.Marshal(full)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal into map failed: %v", err)
	}

	if m[WireTsRFC3339] != "2026-10-30T10:00:00Z" {
		t.Errorf("expected %s to be set, got %v", WireTsRFC3339, m[WireTsRFC3339])
	}
	if m[WireCorrelationID] != "corr-alpha-100" {
		t.Errorf("expected %s to be set, got %v", WireCorrelationID, m[WireCorrelationID])
	}
	if m[WireJobID] != "job-zeta-200" {
		t.Errorf("expected %s to be set, got %v", WireJobID, m[WireJobID])
	}
	if m[objects.FieldKeySource] != DefaultSourceCLI {
		t.Errorf("expected source to be cli, got %v", m[objects.FieldKeySource])
	}
	if m[objects.FieldKeyNote] != "human verification complete" {
		t.Errorf("expected note to match, got %v", m[objects.FieldKeyNote])
	}
	if m[objects.FieldKeyConvergenceSessionRef] != "cvs-stream-01" {
		t.Errorf("expected convergence_session_ref to match, got %v", m[objects.FieldKeyConvergenceSessionRef])
	}

	// Minimal record (empty optional fields)
	minimal := &Record{
		TsRFC3339:     "2026-10-30T10:00:00Z",
		SchemaVersion: SchemaVersionV1,
		EventType:     "minimal_event",
	}

	minData, err := json.Marshal(minimal)
	if err != nil {
		t.Fatalf("MarshalJSON minimal failed: %v", err)
	}

	var minMap map[string]any
	if err := json.Unmarshal(minData, &minMap); err != nil {
		t.Fatalf("unmarshal minimal map failed: %v", err)
	}

	// Optional fields should not be present
	for _, key := range []string{
		objects.FieldKeySource,
		WireCorrelationID,
		WireJobID,
		objects.FieldKeyNote,
		objects.FieldKeyCriteriaRefs,
		objects.FieldKeyBacklogItemRefs,
		objects.FieldKeyConvergenceSessionRef,
		objects.FieldKeyPayload,
	} {
		if _, exists := minMap[key]; exists {
			t.Errorf("key %q should be omitted when empty, but was found in %s", key, string(minData))
		}
	}
}

func TestRecord_UnmarshalJSON_EdgeCases(t *testing.T) {
	// 1. Invalid JSON passed directly to UnmarshalJSON
	var rDirect Record
	if err := rDirect.UnmarshalJSON([]byte(`{not json}`)); err == nil {
		t.Fatalf("expected error on invalid JSON, got nil")
	}
	var r Record
	if err := json.Unmarshal([]byte(`{"other": 1`), &r); err == nil {
		t.Fatalf("expected error on invalid JSON, got nil")
	}

	// 2. Null and empty values for all string/slice/payload fields
	nullJSON := []byte(`{
		"ts_rfc3339": null,
		"schema_version": null,
		"event_type": null,
		"source": null,
		"correlation_id": null,
		"job_id": null,
		"note": null,
		"criteria_refs": null,
		"backlog_item_refs": null,
		"convergence_session_ref": null,
		"payload": null
	}`)
	var rNull Record
	if err := json.Unmarshal(nullJSON, &rNull); err != nil {
		t.Fatalf("unexpected error unmarshaling null fields: %v", err)
	}
	if rNull.TsRFC3339 != "" || rNull.EventType != "" || rNull.Payload != nil || rNull.CriteriaRefs != nil {
		t.Fatalf("expected all fields empty or nil, got %+v", rNull)
	}

	// 3. Fallback to legacy convergence session ID
	legacyCVSJSON := []byte(`{
		"convergence_session_id": "cvs-legacy-99"
	}`)
	var rLegacyCVS Record
	if err := json.Unmarshal(legacyCVSJSON, &rLegacyCVS); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rLegacyCVS.ConvergenceSessionRef != "cvs-legacy-99" {
		t.Fatalf("expected fallback to convergence_session_id, got %q", rLegacyCVS.ConvergenceSessionRef)
	}

	// 4. Prefer canonical convergence_session_ref over legacy key
	bothCVSJSON := []byte(`{
		"convergence_session_ref": "cvs-canonical-1",
		"convergence_session_id": "cvs-legacy-2"
	}`)
	var rBothCVS Record
	if err := json.Unmarshal(bothCVSJSON, &rBothCVS); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rBothCVS.ConvergenceSessionRef != "cvs-canonical-1" {
		t.Fatalf("expected canonical cvs, got %q", rBothCVS.ConvergenceSessionRef)
	}

	// 5. Prefer canonical backlog_item_refs over legacy backlog_item_ids
	bothBLIJSON := []byte(`{
		"backlog_item_refs": ["bli-primary"],
		"backlog_item_ids": ["bli-secondary"]
	}`)
	var rBothBLI Record
	if err := json.Unmarshal(bothBLIJSON, &rBothBLI); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rBothBLI.BacklogItemRefs) != 1 || rBothBLI.BacklogItemRefs[0] != "bli-primary" {
		t.Fatalf("expected canonical backlog_item_refs, got %#v", rBothBLI.BacklogItemRefs)
	}

	// 6. Fallback decodeStrSlice when first key is invalid type
	malformedSliceJSON := []byte(`{
		"backlog_item_refs": "not-an-array",
		"backlog_item_ids": ["bli-fallback-1"]
	}`)
	var rMalformedSlice Record
	if err := json.Unmarshal(malformedSliceJSON, &rMalformedSlice); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rMalformedSlice.BacklogItemRefs) != 1 || rMalformedSlice.BacklogItemRefs[0] != "bli-fallback-1" {
		t.Fatalf("expected fallback slice, got %#v", rMalformedSlice.BacklogItemRefs)
	}

	// 7. Payload contains invalid JSON structure (e.g. number instead of object)
	badPayloadJSON := []byte(`{
		"payload": 12345
	}`)
	var rBadPayload Record
	if err := json.Unmarshal(badPayloadJSON, &rBadPayload); err == nil {
		t.Fatalf("expected error unmarshaling non-object payload, got nil")
	}
}

func TestAppend_ConcurrentSafety(t *testing.T) {
	root := t.TempDir()
	numWorkers := 20
	iterationsPerWorker := 15
	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		workerID := w
		go func() {
			defer wg.Done()
			for i := 0; i < iterationsPerWorker; i++ {
				rec := &Record{
					EventType:     "concurrent_tick",
					Source:        DefaultSourceCLI,
					CorrelationID: fmt.Sprintf("corr-w%d-i%d", workerID, i),
					JobID:         fmt.Sprintf("job-%d", workerID),
					Note:          fmt.Sprintf("worker %d iteration %d", workerID, i),
					CriteriaRefs:  []string{fmt.Sprintf("crit-w%d", workerID)},
					Payload: map[string]any{
						"worker": float64(workerID),
						"iter":   float64(i),
					},
				}
				if err := Append(root, rec); err != nil {
					t.Errorf("worker %d iteration %d Append failed: %v", workerID, i, err)
				}
			}
		}()
	}

	wg.Wait()

	// Verify all records were written as valid JSON lines
	path := ContextEventsJSONLPath(root)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed reading written file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	expectedCount := numWorkers * iterationsPerWorker
	if len(lines) != expectedCount {
		t.Fatalf("expected %d lines, got %d", expectedCount, len(lines))
	}

	for idx, line := range lines {
		var r Record
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("line %d is not valid Record JSON: %v, content=%q", idx, err, line)
		}
		if r.EventType != "concurrent_tick" || r.SchemaVersion != SchemaVersionV1 {
			t.Fatalf("line %d has unexpected fields: %+v", idx, r)
		}
	}
}

func TestTruncateNote_Branches(t *testing.T) {
	// Empty string
	if got := truncateNote(""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	// Under limit
	if got := truncateNote("short"); got != "short" {
		t.Fatalf("expected short, got %q", got)
	}
}

func TestTrimSlice_Branches(t *testing.T) {
	// Nil
	if got := trimSlice(nil); got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
	// Empty
	if got := trimSlice([]string{}); got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
	// All spaces
	if got := trimSlice([]string{" ", "\t", "  "}); got != nil {
		t.Fatalf("expected nil, got %#v", got)
	}
	// Mixed
	got := trimSlice([]string{"  a ", " ", "b  "})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected trimSlice result: %#v", got)
	}
}
