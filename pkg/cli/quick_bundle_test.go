package cli

import (
	"fmt"
	"os"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestQuickBundle(t *testing.T) {
	// Satisfies [REDACTED-ID]
	// Satisfies [REDACTED-ID]
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
