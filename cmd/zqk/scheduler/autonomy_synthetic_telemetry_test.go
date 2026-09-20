package scheduler

import (
	"encoding/json"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestAutonomyLiveTelemetryJSON(t *testing.T) {
	t.Parallel()

	raw := autonomyTelemetryJSON("")
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, raw)
	}
	syn, ok := payload["synthetic"].(bool)
	if !ok || syn {
		t.Fatalf("synthetic marker must be false: %#v", payload)
	}
	if available, _ := payload["available"].(bool); !available {
		t.Fatalf("available must be true: %#v", payload)
	}
	if status, _ := payload[objects.FieldKeyStatus].(string); status != "ok" {
		t.Fatalf("status must be ok: %#v", payload)
	}
}
