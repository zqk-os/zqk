package translation

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestDefaultEngine_Translate_JSON(t *testing.T) {
	engine := NewDefaultEngine()
	ctx := context.Background()

	input := []byte(`{"name": "my-tool", "description": "does things"}`)

	obj, err := engine.Translate(ctx, input, "json", "my-tool.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if obj[objects.FieldKeyKind] != "tool" {
		t.Errorf("expected kind 'tool', got %v", obj[objects.FieldKeyKind])
	}
	if obj[objects.FieldKeyID] != "ext-my-tool" {
		t.Errorf("expected id 'ext-my-tool', got %v", obj[objects.FieldKeyID])
	}
	if obj[objects.FieldKeyName] != "my-tool" {
		t.Errorf("expected name 'my-tool', got %v", obj[objects.FieldKeyName])
	}
}

func TestDefaultEngine_Translate_YAML(t *testing.T) {
	engine := NewDefaultEngine()
	ctx := context.Background()

	input := []byte(`
name: my-skill
instructions: "do things"
`)

	obj, err := engine.Translate(ctx, input, "yaml", "my-skill.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if obj[objects.FieldKeyKind] != "skill" {
		t.Errorf("expected kind 'skill', got %v", obj[objects.FieldKeyKind])
	}
	if obj[objects.FieldKeyID] != "ext-my-skill" {
		t.Errorf("expected id 'ext-my-skill', got %v", obj[objects.FieldKeyID])
	}
	if obj[objects.FieldKeyName] != "my-skill" {
		t.Errorf("expected name 'my-skill', got %v", obj[objects.FieldKeyName])
	}
}

func TestDefaultEngine_Translate_UnsupportedFormat(t *testing.T) {
	engine := NewDefaultEngine()
	ctx := context.Background()

	_, err := engine.Translate(ctx, []byte("some text"), "txt", "test.txt")
	if err == nil {
		t.Fatal("expected error for unsupported format, got nil")
	}
}
