package datacell

import (
	"testing"
)

func TestStewardshipRegistry(t *testing.T) {
	reg := NewStewardshipRegistry()
	reg.UpdateCount("test_kind", 42)

	count, ok := reg.GetCount("test_kind")
	if !ok || count != 42 {
		t.Errorf("Expected count 42, got %d", count)
	}
}
