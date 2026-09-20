package audit

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestMetricUpdateFields(t *testing.T) {
	t.Parallel()
	got := MetricUpdateFields(map[string]any{
		objects.FieldKeyID:        "AAM-1",
		objects.FieldKeyCreatedAt: "old",
		objects.FieldKeyCreatedBy: "alice",
		objects.FieldKeyTitle:     "keep",
	}, "now", "system")
	if _, ok := got[objects.FieldKeyID]; ok {
		t.Fatal("id should be dropped")
	}
	if got[objects.FieldKeyTitle] != "keep" || got[objects.FieldKeyUpdatedAt] != "now" || got[objects.FieldKeyUpdatedBy] != "system" {
		t.Fatalf("%v", got)
	}
}
