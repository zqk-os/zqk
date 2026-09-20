package internal

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestGroupResults_SingleField(t *testing.T) {
	// Setup test data
	obj1 := map[string]any{objects.FieldKeyID: "id1", objects.FieldKeyStatus: "active"}
	obj2 := map[string]any{objects.FieldKeyID: "id2", objects.FieldKeyStatus: "draft"}
	obj3 := map[string]any{objects.FieldKeyID: "id3", objects.FieldKeyStatus: "active"}

	result := &storage.QueryResult{
		Objects: []map[string]any{obj1, obj2, obj3},
	}

	grouped := groupResults(result, "status")

	if len(grouped.Groups) != 2 {
		t.Fatalf("Expected 2 groups, got %d", len(grouped.Groups))
	}

	if len(grouped.Groups["active"]) != 2 {
		t.Errorf("Expected 2 objects in 'active' group, got %d", len(grouped.Groups["active"]))
	}

	if len(grouped.Groups["draft"]) != 1 {
		t.Errorf("Expected 1 object in 'draft' group, got %d", len(grouped.Groups["draft"]))
	}
}

func TestGroupResults_MultiField(t *testing.T) {
	// Setup test data
	obj1 := map[string]any{objects.FieldKeyID: "id1", objects.FieldKeyStatus: "active", objects.FieldKeyPriority: "high"}
	obj2 := map[string]any{objects.FieldKeyID: "id2", objects.FieldKeyStatus: "draft", objects.FieldKeyPriority: "low"}
	obj3 := map[string]any{objects.FieldKeyID: "id3", objects.FieldKeyStatus: "active", objects.FieldKeyPriority: "high"}
	obj4 := map[string]any{objects.FieldKeyID: "id4", objects.FieldKeyStatus: "active", objects.FieldKeyPriority: "low"}

	result := &storage.QueryResult{
		Objects: []map[string]any{obj1, obj2, obj3, obj4},
	}

	grouped := groupResults(result, "status,priority")

	if len(grouped.Groups) != 3 {
		t.Fatalf("Expected 3 groups, got %d", len(grouped.Groups))
	}

	if len(grouped.Groups["active, high"]) != 2 {
		t.Errorf("Expected 2 objects in 'active, high' group, got %d", len(grouped.Groups["active, high"]))
	}

	if len(grouped.Groups["draft, low"]) != 1 {
		t.Errorf("Expected 1 object in 'draft, low' group, got %d", len(grouped.Groups["draft, low"]))
	}

	if len(grouped.Groups["active, low"]) != 1 {
		t.Errorf("Expected 1 object in 'active, low' group, got %d", len(grouped.Groups["active, low"]))
	}
}

func TestGroupResults_MissingField(t *testing.T) {
	// Setup test data where a field might be missing
	obj1 := map[string]any{objects.FieldKeyID: "id1", objects.FieldKeyStatus: "active"} // missing priority
	obj2 := map[string]any{objects.FieldKeyID: "id2", objects.FieldKeyStatus: "draft", objects.FieldKeyPriority: "low"}

	result := &storage.QueryResult{
		Objects: []map[string]any{obj1, obj2},
	}

	grouped := groupResults(result, "status,priority")

	if len(grouped.Groups) != 2 {
		t.Fatalf("Expected 2 groups, got %d", len(grouped.Groups))
	}

	if len(grouped.Groups["active, (none)"]) != 1 {
		t.Errorf("Expected 1 object in 'active, (none)' group, got %d", len(grouped.Groups["active, (none)"]))
	}
}
