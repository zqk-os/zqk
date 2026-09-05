package instance_builders

import (
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestBaseInstanceBuilder_BuildRequiredTemplate(t *testing.T) {
	t.Parallel()

	builder := NewBaseInstanceBuilder("test_case", "2.0.0", nil, nil)
	templateMap, err := builder.BuildRequiredTemplate()
	if err != nil {
		t.Fatalf("BuildRequiredTemplate() failed: %v", err)
	}

	if got, ok := templateMap[objects.FieldKeyKind].(string); !ok || got != "test_case" {
		t.Fatalf("kind missing or incorrect: %#v", templateMap[objects.FieldKeyKind])
	}
	if got, ok := templateMap[objects.FieldKeySchemaVersion].(string); !ok || got != "2.0.0" {
		t.Fatalf("schema_version missing or incorrect: %#v", templateMap[objects.FieldKeySchemaVersion])
	}
	if got, ok := templateMap[objects.FieldKeyID].(string); !ok || got == emptyValue {
		t.Fatalf("id missing or incorrect: %#v", templateMap[objects.FieldKeyID])
	}
}

func TestBaseInstanceBuilder_BuildRequiredTemplateFormats(t *testing.T) {
	t.Parallel()

	builder := NewBaseInstanceBuilder("test_case", "2.0.0", nil, nil)
	yamlOut, err := builder.BuildRequiredTemplateYAML()
	if err != nil {
		t.Fatalf("BuildRequiredTemplateYAML() failed: %v", err)
	}
	if !strings.Contains(yamlOut, "kind: test_case") {
		t.Fatalf("yaml output missing kind: %s", yamlOut)
	}

	jsonOut, err := builder.BuildRequiredTemplateJSON()
	if err != nil {
		t.Fatalf("BuildRequiredTemplateJSON() failed: %v", err)
	}
	if !strings.Contains(jsonOut, "\"kind\": \"test_case\"") {
		t.Fatalf("json output missing kind: %s", jsonOut)
	}
}
