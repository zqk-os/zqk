package object

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNewTemplateCmd_Structure(t *testing.T) {
	cmd := NewTemplateCmd()
	if cmd == nil {
		t.Fatal("expected NewTemplateCmd to return non-nil")
	}
	if cmd.Use == "" {
		t.Error("expected Use to be set")
	}
	if !strings.Contains(cmd.Long, "Canonical draft path") {
		t.Errorf("expected Long to mention Canonical draft path, got: %s", cmd.Long)
	}
}

func TestInProcess_Template_StdoutAndFile(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	// 1. Generate template to stdout for backlog_item
	out, err := executeObjectCommand(t, tempDir, provider, "template", "backlog_item")
	if err != nil {
		t.Fatalf("template backlog_item failed: %v", err)
	}
	if !strings.Contains(out, "kind: backlog_item") {
		t.Errorf("expected template output to contain 'kind: backlog_item', got:\n%s", out)
	}
	if !strings.Contains(out, "schema_version: \"2.0.0\"") {
		t.Errorf("expected schema_version 2.0.0, got:\n%s", out)
	}
	if !strings.Contains(out, "# Required fields") {
		t.Errorf("expected '# Required fields' in template output, got:\n%s", out)
	}

	// 2. Generate template with explicit stdout "-"
	outStdoutFlag, err := executeObjectCommand(t, tempDir, provider, "template", "backlog_item", "--output", "-")
	if err != nil {
		t.Fatalf("template backlog_item --output - failed: %v", err)
	}
	if !strings.Contains(outStdoutFlag, "kind: backlog_item") {
		t.Errorf("expected stdout flag template output to contain 'kind: backlog_item', got:\n%s", outStdoutFlag)
	}

	// 3. Generate template to file
	outFile := filepath.Join(tempDir, "bli_template.yaml")
	outFileMsg, err := executeObjectCommand(t, tempDir, provider, "template", "backlog_item", "--output", outFile)
	if err != nil {
		t.Fatalf("template backlog_item --output file failed: %v", err)
	}
	if !strings.Contains(outFileMsg, "Template written to:") {
		t.Errorf("expected success message with file path, got: %s", outFileMsg)
	}

	fileContent, err := fileutil.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read written template file: %v", err)
	}
	if !strings.Contains(string(fileContent), "kind: backlog_item") {
		t.Errorf("expected file to contain template YAML, got:\n%s", string(fileContent))
	}

	// 4. Error path: Output to unwritable path
	badOutFile := filepath.Join(tempDir, "nonexistent_dir", "sub", "cannot_write.yaml")
	_, errBadFile := executeObjectCommand(t, tempDir, provider, "template", "backlog_item", "--output", badOutFile)
	if errBadFile == nil {
		t.Errorf("expected error when writing template to unwritable directory path")
	}

	// 5. Error path: Invalid kind
	_, errInvalidKind := executeObjectCommand(t, tempDir, provider, "template", "nonexistent_kind_xyz")
	if errInvalidKind == nil {
		t.Errorf("expected error for nonexistent kind")
	}

	// 6. Test various kinds
	for _, kind := range []string{"goal", "priority_plan", "criteria", "milestone"} {
		kOut, kErr := executeObjectCommand(t, tempDir, provider, "template", kind)
		if kErr != nil {
			t.Errorf("template %s failed: %v", kind, kErr)
			continue
		}
		if !strings.Contains(kOut, "kind: "+kind) {
			t.Errorf("expected template for %s to contain 'kind: %s', got:\n%s", kind, kind, kOut)
		}
		if !strings.Contains(kOut, "schema_version: \"2.0.0\"") {
			t.Errorf("expected template for %s to contain schema_version 2.0.0", kind)
		}
	}
}

func TestWriteCommentBlock_Multiline(t *testing.T) {
	var b strings.Builder
	writeCommentBlock(&b, "First line\n\nThird line")
	res := b.String()
	expected := "# First line\n#\n# Third line\n"
	if res != expected {
		t.Errorf("expected comment block:\n%q\ngot:\n%q", expected, res)
	}
}

func TestGetPlaceholderValue_Comprehensive(t *testing.T) {
	// Status field with origin status
	val := getPlaceholderValue(&objects.FieldInfo{Name: "status", Type: "enum"}, "proposed")
	if val != "\"proposed\"" {
		t.Errorf("expected \"proposed\", got %s", val)
	}

	// Standard audit fields
	if getPlaceholderValue(&objects.FieldInfo{Name: objects.FieldKeyCreatedAt}, "") != "\"2026-01-01T00:00:00Z\"" {
		t.Error("unexpected created_at placeholder")
	}
	if getPlaceholderValue(&objects.FieldInfo{Name: objects.FieldKeyUpdatedAt}, "") != "\"2026-01-01T00:00:00Z\"" {
		t.Error("unexpected updated_at placeholder")
	}
	if !strings.Contains(getPlaceholderValue(&objects.FieldInfo{Name: objects.FieldKeyCreatedBy}, ""), "ACC-SYSTEM") {
		t.Error("unexpected created_by placeholder")
	}
	if !strings.Contains(getPlaceholderValue(&objects.FieldInfo{Name: objects.FieldKeyUpdatedBy}, ""), "ACC-SYSTEM") {
		t.Error("unexpected updated_by placeholder")
	}

	// Types
	cases := []struct {
		field    objects.FieldInfo
		expected string
	}{
		{objects.FieldInfo{Type: "string"}, "\"\""},
		{objects.FieldInfo{Type: "text"}, "\"\""},
		{objects.FieldInfo{Type: "integer"}, "0"},
		{objects.FieldInfo{Type: "number"}, "0"},
		{objects.FieldInfo{Type: "boolean"}, "false"},
		{objects.FieldInfo{Type: "list"}, "[]"},
		{objects.FieldInfo{Type: "map"}, "{}"},
		{objects.FieldInfo{Type: "object"}, "{}"},
		{objects.FieldInfo{Type: "enum", EnumValues: []string{"val1", "val2"}}, "\"val1\""},
		{objects.FieldInfo{Type: "enum"}, "\"\""},
		{objects.FieldInfo{Type: "reference", SemanticType: "requirement"}, "\"# requirement reference (e.g., requirement:ID-###)\""},
		{objects.FieldInfo{Type: "reference"}, "\"\""},
		{objects.FieldInfo{Type: "unknown_custom_type"}, "\"\""},
	}

	for _, tc := range cases {
		got := getPlaceholderValue(&tc.field, "")
		if got != tc.expected {
			t.Errorf("for type %s: expected %s, got %s", tc.field.Type, tc.expected, got)
		}
	}
}

func TestWriteFieldTemplate_Variations(t *testing.T) {
	var b strings.Builder

	// Status with all status values
	fStatus := objects.FieldInfo{
		Name:        "status",
		Type:        "enum",
		Description: "The status of the item",
		Required:    true,
	}
	writeFieldTemplate(&b, &fStatus, true, "draft", []string{"draft", "active", "completed"})
	res := b.String()
	if !strings.Contains(res, "Valid values: \"draft\", \"active\", \"completed\"") {
		t.Errorf("expected valid values in status field template, got:\n%s", res)
	}

	// Optional enum field
	b.Reset()
	fEnum := objects.FieldInfo{
		Name:        "priority",
		Type:        "enum",
		EnumValues:  []string{"p0", "p1", "p2"},
		Description: "Priority level",
		Required:    false,
	}
	writeFieldTemplate(&b, &fEnum, false, "", nil)
	res = b.String()
	if !strings.Contains(res, "[optional]") || !strings.Contains(res, "Valid values: \"p0\", \"p1\", \"p2\"") {
		t.Errorf("expected optional enum comments, got:\n%s", res)
	}

	// Field without description but with type (optional)
	b.Reset()
	fNoDesc := objects.FieldInfo{
		Name:     "custom_tag",
		Type:     "string",
		Required: false,
	}
	writeFieldTemplate(&b, &fNoDesc, false, "", nil)
	res = b.String()
	if !strings.Contains(res, "# (type: string) [optional]") {
		t.Errorf("expected type and optional tag, got:\n%s", res)
	}
}
