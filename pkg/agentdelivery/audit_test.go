package agentdelivery

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
)

func TestAgentPromptDeliveriesJSONLPath_emptyRoot(t *testing.T) {
	p := AgentPromptDeliveriesJSONLPath("")
	if p == "" {
		t.Fatal("expected non-empty path")
	}
	if filepath.Base(p) != AgentPromptDeliveriesJSONLFile {
		t.Errorf("expected base %q, got %q", AgentPromptDeliveriesJSONLFile, filepath.Base(p))
	}
}

func TestAgentPromptDeliveriesJSONLPath_customRoot(t *testing.T) {
	base := "/my/project/root"
	p := AgentPromptDeliveriesJSONLPath(base)
	if p == "" {
		t.Fatal("expected non-empty path for custom root")
	}
	if filepath.Base(p) != AgentPromptDeliveriesJSONLFile {
		t.Errorf("expected base %q, got %q", AgentPromptDeliveriesJSONLFile, filepath.Base(p))
	}
}

func TestDeliveryAuditRecord_NilReceiver(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("AppendDeliveryAuditJSONL with nil record panicked: %v", r)
		}
	}()
	AppendDeliveryAuditJSONL("", nil)
}

func TestDeliveryAuditRecord_MarshalRoundTrip(t *testing.T) {
	rec := &DeliveryAuditRecord{
		Timestamp:            "2026-01-01T00:00:00Z",
		EventType:            EventTypeAgentPromptDelivery,
		ConvergenceSessionID: "conv-001",
		Format:               "markdown",
		DeliverMode:          "local",
		AttentionMode:        "direct",
		OutputPath:           "/tmp/output.md",
		PrimaryDestination:   "terminal",
		DeliveredTo:          []string{"agent-1"},
		HTTPURL:              "http://example.com/hook",
		HTTPStatusCode:       200,
		MarkdownBytes:        42,
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty marshaled data")
	}

	var got DeliveryAuditRecord
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if got.PrimaryDestination != "terminal" {
		t.Errorf("expected PrimaryDestination='terminal', got %q", got.PrimaryDestination)
	}
}

func TestDeliveryAuditRecord_EmptyFieldsJSON(t *testing.T) {
	// Fields omitted when empty should not appear in JSON.
	rec := &DeliveryAuditRecord{
		Timestamp:            "2026-01-01T00:00:00Z",
		EventType:            EventTypeAgentPromptDelivery,
		ConvergenceSessionID: "conv-001",
		PrimaryDestination:   "terminal",
		DeliveredTo:          []string{},
	}

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal to map failed: %v", err)
	}

	// AttentionMode and HTTPURL, HTTPError, OutputPath are omitempty — should not appear.
	for _, key := range []string{"attention_mode", "http_url", "http_error", "output_path"} {
		if _, ok := raw[key]; ok {
			t.Errorf("expected key %q to be omitted from JSON, but it was present", key)
		}
	}

	// MarkdownBytes with value 0 should appear.
	if _, ok := raw["markdown_bytes"]; !ok {
		t.Error("expected 'markdown_bytes' key in JSON")
	}
}

func TestAgentPromptDeliveriesJSONLPath_pathComponents(t *testing.T) {
	// The path must be under paths.ProjectDataDir/paths.LogsDir/paths.SchedulerJobLogsSubdir.
	p := AgentPromptDeliveriesJSONLPath("/my/project/root")
	if !filepath.IsAbs(p) {
		t.Fatal("expected absolute path")
	}

	// Verify the path contains expected subdirectory components.
	components := []string{
		paths.ProjectDataDir,
		paths.LogsDir,
	}

	for _, comp := range components {
		if !containsSubstr(p, comp) {
			t.Errorf("expected path %q to contain component %q", p, comp)
		}
	}
}

func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || hasSubstring(s, substr))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
