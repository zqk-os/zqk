package scheduler

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestAppendCVSMeasurementEvent_roundTrip(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	entry := map[string]any{
		KeyCVSEventType:           CVSEventTypeMeasureApplied,
		objects.FieldKeySessionID: "CONV-test",
		objects.FieldKeyTickJobID: "SCH-tick-1",
		objects.FieldKeyWatermark: "2026-04-16T12:00:00Z",
	}
	AppendCVSMeasurementEvent(root, entry)
	path := CVSMeasurementEventsFilePath(root)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	line := strings.TrimSpace(string(b))
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got[objects.FieldKeySessionID] != "CONV-test" {
		t.Fatalf("session_id: %v", got[objects.FieldKeySessionID])
	}
}

func TestCvsMeasurementEventsEnabled(t *testing.T) {
	t.Parallel()
	if !cvsMeasurementEventsEnabled(&ScheduledJob{EnvironmentVariables: map[string]string{}}) {
		t.Fatal("empty env should enable")
	}
	if !cvsMeasurementEventsEnabled(&ScheduledJob{EnvironmentVariables: map[string]string{EnvKeyCVSMeasurementEvents: "1"}}) {
		t.Fatal("1 should enable")
	}
	if cvsMeasurementEventsEnabled(&ScheduledJob{EnvironmentVariables: map[string]string{EnvKeyCVSMeasurementEvents: "0"}}) {
		t.Fatal("0 should disable")
	}
	if cvsMeasurementEventsEnabled(&ScheduledJob{EnvironmentVariables: map[string]string{EnvKeyCVSMeasurementEvents: "false"}}) {
		t.Fatal("false should disable")
	}
}

func TestTruncateCVSHint(t *testing.T) {
	t.Parallel()
	s := strings.Repeat("あ", 10)
	got := truncateCVSHint(s, 3)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis suffix, got %q", got)
	}
}
