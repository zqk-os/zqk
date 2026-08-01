package mcp

import (
	"encoding/json"
	"testing"
)

// TestResourcesListResultNeverMarshalsNull ensures resources/list returns [] not null
// (ITEM-EXAMPLE / Cursor schema validation).
func TestResourcesListResultNeverMarshalsNull(t *testing.T) {
	t.Parallel()
	var resources []Resource // nil slice
	if resources == nil {
		resources = []Resource{}
	}
	payload := map[string]any{"resources": resources}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != `{"resources":[]}` {
		t.Fatalf("expected empty JSON array, got %s", string(b))
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	raw := string(decoded["resources"])
	if raw == "null" || raw == "" {
		t.Fatalf("resources must not be null, got %q", raw)
	}
}
