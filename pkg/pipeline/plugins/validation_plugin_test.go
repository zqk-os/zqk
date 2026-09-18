package plugins

import (
	"context"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestValidationPlugin_Execute(t *testing.T) {
	plugin := NewValidationPlugin([]string{"result", "status"})

	t.Run("Compliant payload", func(t *testing.T) {
		payload := map[string]any{
			"result":               "success",
			objects.FieldKeyStatus: "completed",
		}
		out, err := plugin.Execute(context.Background(), payload)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if out["validation_status"] != "passed" {
			t.Errorf("expected validation_status to be passed, got %v", out["validation_status"])
		}
	})

	t.Run("Non-compliant payload", func(t *testing.T) {
		payload := map[string]any{
			objects.FieldKeyStatus: "completed",
		}
		out, err := plugin.Execute(context.Background(), payload)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if out["validation_status"] != "failed" {
			t.Errorf("expected validation_status to be failed, got %v", out["validation_status"])
		}
		if out["validation_errors"] == nil {
			t.Errorf("expected validation_errors to be set")
		} else if !strings.Contains(out["validation_errors"].(string), "missing required fields: [result]") {
			t.Errorf("unexpected validation_errors: %v", out["validation_errors"])
		}
	})

	t.Run("Nil payload", func(t *testing.T) {
		out, err := plugin.Execute(context.Background(), nil)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if out["validation_status"] != "failed" {
			t.Errorf("expected validation_status to be failed, got %v", out["validation_status"])
		}
	})
}

func TestValidationPlugin_Validate(t *testing.T) {
	plugin := NewValidationPlugin([]string{})

	t.Run("Valid status", func(t *testing.T) {
		output := map[string]any{
			"validation_status": "passed",
		}
		err := plugin.Validate(context.Background(), output)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("Invalid status", func(t *testing.T) {
		output := map[string]any{
			"validation_status": "failed",
			"validation_errors": "missing required fields: [result]",
		}
		err := plugin.Validate(context.Background(), output)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "missing required fields: [result]") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("Missing status", func(t *testing.T) {
		output := map[string]any{}
		err := plugin.Validate(context.Background(), output)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
	})
}

func TestValidationPlugin_Name(t *testing.T) {
	plugin := NewValidationPlugin(nil)
	if name := plugin.Name(); name != "ValidationPlugin" {
		t.Errorf("expected ValidationPlugin, got %s", name)
	}
}
