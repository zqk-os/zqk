package cli

import (
	"fmt"
	"os"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestQuickBundle(t *testing.T) {
	// Satisfies core-backlog
	// Satisfies core-backlog
	bundleContent := []byte(`
backlog_item:
  title: Test BLI
criteria:
  - title: Crit 1
agent_tasks:
  - title: Task 1
`)

	var ids int
	mockCreator := func(kind string, data map[string]any) (string, error) {
		ids++
		id := fmt.Sprintf("%s-%d", kind, ids)
		data[objects.FieldKeyID] = id
		return id, nil
	}

	err := ProcessQuickBundle(bundleContent, mockCreator, os.Stdout, nil)
	if err != nil {
		t.Fatalf("ProcessQuickBundle failed: %v", err)
	}

	if ids != 3 {
		t.Fatalf("Expected 3 objects created, got %d", ids)
	}
}

func TestAppendRef(t *testing.T) {
	t.Parallel()

	t.Run("nil_field", func(t *testing.T) {
		m := make(map[string]any)
		appendRef(m, "refs", "REF-1")
		slice, ok := m["refs"].([]any)
		if !ok || len(slice) != 1 || slice[0] != "REF-1" {
			t.Fatalf("expected [REF-1], got %v", m["refs"])
		}
	})

	t.Run("any_slice", func(t *testing.T) {
		m := map[string]any{
			"refs": []any{"REF-1"},
		}
		appendRef(m, "refs", "REF-2")
		slice, ok := m["refs"].([]any)
		if !ok || len(slice) != 2 || slice[1] != "REF-2" {
			t.Fatalf("expected [REF-1, REF-2], got %v", m["refs"])
		}
	})

	t.Run("string_slice", func(t *testing.T) {
		m := map[string]any{
			"refs": []string{"REF-1"},
		}
		appendRef(m, "refs", "REF-2")
		slice, ok := m["refs"].([]any)
		if !ok || len(slice) != 2 || slice[0] != "REF-1" || slice[1] != "REF-2" {
			t.Fatalf("expected converted []any [REF-1, REF-2], got %v", m["refs"])
		}
	})
}

