package plugins

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestAuditLoggingPlugin_Name(t *testing.T) {
	p := NewAuditLoggingPlugin(nil)
	if p.Name() != "AuditLoggingPlugin" {
		t.Errorf("expected Name to be AuditLoggingPlugin, got %s", p.Name())
	}
}

func TestAuditLoggingPlugin_Execute_Success(t *testing.T) {
	var buf bytes.Buffer
	p := NewAuditLoggingPlugin(&buf)
	ctx := context.Background()

	payload := map[string]any{
		"key1": "value1",
	}

	result, err := p.Execute(ctx, payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if logged, ok := result["audit_logged"].(bool); !ok || !logged {
		t.Errorf("expected payload to have audit_logged = true")
	}

	logStr := buf.String()
	if !strings.Contains(logStr, "\"pipeline_activity\"") {
		t.Errorf("expected log to contain \"pipeline_activity\", got %s", logStr)
	}
	if !strings.Contains(logStr, "\"key1\":\"value1\"") {
		t.Errorf("expected log to contain payload keys, got %s", logStr)
	}
}

func TestAuditLoggingPlugin_Validate_Success(t *testing.T) {
	p := NewAuditLoggingPlugin(nil)
	ctx := context.Background()
	output := map[string]any{
		"audit_logged": true,
	}
	err := p.Validate(ctx, output)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuditLoggingPlugin_Validate_Failure(t *testing.T) {
	p := NewAuditLoggingPlugin(nil)
	ctx := context.Background()
	output := map[string]any{}
	err := p.Validate(ctx, output)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	output["audit_logged"] = false
	err = p.Validate(ctx, output)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}
