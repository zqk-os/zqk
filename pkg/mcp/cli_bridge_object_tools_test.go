package mcp

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

func TestZqkObjectCreateSchema(t *testing.T) {
	// Create mock object create command
	cmd := &cobra.Command{
		Use:   "create <kind>",
		Short: "Create object",
	}
	cmd.Flags().String("file", "", "file path")
	cmd.Flags().StringArray("field", []string{}, "fields")

	discovered := &DiscoveredCommand{
		Use:     cmd.Use,
		Short:   cmd.Short,
		Command: cmd,
		Path:    "object create",
	}

	tool := ConvertCommandToMCPTool(discovered)

	// Schema should have id and fields properties added dynamically
	schemaMap := tool.InputSchema.(map[string]any)
	props := schemaMap["properties"].(map[string]any)

	if _, ok := props[objects.FieldKeyID]; !ok {
		t.Error("Expected 'id' in schema properties")
	}
	if _, ok := props["fields"]; !ok {
		t.Error("Expected 'fields' in schema properties")
	}
}

func TestExtractFlagsForObjectCreate(t *testing.T) {
	args := map[string]any{
		"_command_path":      "object create",
		objects.FieldKeyID:   "BL-123",
		objects.FieldKeyKind: "backlog_item",
		"fields": map[string]any{
			objects.FieldKeyTitle: "Test Title",
		},
	}

	flags := ExtractFlags(args, nil)

	foundId := false
	foundTitle := false

	for i, f := range flags {
		if f == "--field" && i+1 < len(flags) {
			if flags[i+1] == "id=BL-123" {
				foundId = true
			} else if flags[i+1] == "title=Test Title" {
				foundTitle = true
			}
		}
	}

	if !foundId {
		t.Errorf("missing --field id=BL-123, got %v", flags)
	}
	if !foundTitle {
		t.Errorf("missing --field title=Test Title, got %v", flags)
	}

	posArgs := ExtractPositionalArguments(args, nil)
	if len(posArgs) != 1 || posArgs[0] != "backlog_item" {
		t.Errorf("Expected posArgs [backlog_item], got %v", posArgs)
	}
}
