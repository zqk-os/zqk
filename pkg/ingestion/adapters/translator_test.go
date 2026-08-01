package adapters

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestAgentTranslator_Translate_GeminiSkill(t *testing.T) {
	translator := NewAgentTranslator()
	ctx := context.Background()

	input := []byte(`
name: github-search
instructions: "Search github"
`)

	obj, err := translator.Translate(ctx, input, "yaml", "github-search.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if obj[objects.FieldKeyKind] != "tool_spec" {
		t.Errorf("expected kind 'tool_spec', got %v", obj[objects.FieldKeyKind])
	}
	if obj[objects.FieldKeyID] != "ext-github-search" {
		t.Errorf("expected id 'ext-github-search', got %v", obj[objects.FieldKeyID])
	}
}

func TestAgentTranslator_Translate_CursorRule(t *testing.T) {
	translator := NewAgentTranslator()
	ctx := context.Background()

	input := []byte(`{
		"name": "my-rule",
		"rules": ["no fmt.Print"]
	}`)

	obj, err := translator.Translate(ctx, input, "json", "my-rule.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if obj[objects.FieldKeyKind] != "policy" {
		t.Errorf("expected kind 'policy', got %v", obj[objects.FieldKeyKind])
	}
	if obj[objects.FieldKeyID] != "ext-my-rule" {
		t.Errorf("expected id 'ext-my-rule', got %v", obj[objects.FieldKeyID])
	}
}

func TestAgentTranslator_Translate_UnknownFormat(t *testing.T) {
	translator := NewAgentTranslator()
	ctx := context.Background()

	_, err := translator.Translate(ctx, []byte("bad"), "txt", "bad.txt")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
