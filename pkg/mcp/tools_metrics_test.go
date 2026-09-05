package mcp

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestHandleGetToolMetrics_ErrorRateCalculation(t *testing.T) {
	server := NewServer()

	// Test missing tool_name argument
	_, err := HandleGetToolMetrics(server, map[string]any{})
	if err == nil {
		t.Error("Expected error when tool_name argument is missing")
	}

	// Test non-existent tool metrics
	res, err := HandleGetToolMetrics(server, map[string]any{"tool_name": "non_existent"})
	if err != nil {
		t.Fatalf("Unexpected error for non_existent tool: %v", err)
	}

	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("Expected map response, got %T", res)
	}

	if m["found"] != false {
		t.Errorf("Expected found=false, got %v", m["found"])
	}
}

func TestHandleGetToolMetrics_ZeroCallCountZeroErrorRate(t *testing.T) {
	server := NewServer()
	server.mcpMetrics.RegisterUncalledToolMetrics("test_tool")

	res, err := HandleGetToolMetrics(server, map[string]any{"tool_name": "test_tool"})
	if err != nil {
		t.Fatalf("HandleGetToolMetrics failed: %v", err)
	}

	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("Expected map response, got %T", res)
	}

	if m["found"] != true {
		t.Fatalf("Expected found=true, got %v", m["found"])
	}

	if m["call_count"] != int64(0) {
		t.Errorf("Expected call_count 0, got %v", m["call_count"])
	}

	if m[objects.FieldKeyErrorRate] != float64(0.0) {
		t.Errorf("Expected error_rate 0.0 for CallCount=0, got %v", m[objects.FieldKeyErrorRate])
	}
}

func TestHandleGetMetrics_Format(t *testing.T) {
	server := NewServer()

	resJSON, err := HandleGetMetrics(server, map[string]any{objects.FieldKeyFormat: "json"})
	if err != nil {
		t.Fatalf("HandleGetMetrics(json) error: %v", err)
	}
	if _, ok := resJSON.(MetricsSnapshot); !ok {
		t.Errorf("Expected MetricsSnapshot for json format, got %T", resJSON)
	}

	resSummary, err := HandleGetMetrics(server, map[string]any{objects.FieldKeyFormat: "summary"})
	if err != nil {
		t.Fatalf("HandleGetMetrics(summary) error: %v", err)
	}
	if _, ok := resSummary.(string); !ok {
		t.Errorf("Expected string for summary format, got %T", resSummary)
	}
}

func TestMCPMetrics_RegisterUncalledToolMetrics_Standalone(t *testing.T) {
	metrics := NewMCPMetrics()
	metrics.RegisterUncalledToolMetrics("standalone_tool")

	snapshot := metrics.GetSnapshot()
	toolMetrics, exists := snapshot.Tools.ByTool["standalone_tool"]
	if !exists {
		t.Fatalf("Expected standalone_tool in metrics snapshot")
	}

	if toolMetrics.CallCount != 0 {
		t.Errorf("Expected CallCount=0, got %d", toolMetrics.CallCount)
	}
	if toolMetrics.ErrorCount != 0 {
		t.Errorf("Expected ErrorCount=0, got %d", toolMetrics.ErrorCount)
	}

	metrics.RecordToolCall("standalone_tool", 100, nil)
	metrics.RegisterUncalledToolMetrics("standalone_tool")

	snapshot2 := metrics.GetSnapshot()
	if snapshot2.Tools.ByTool["standalone_tool"].CallCount != 1 {
		t.Errorf("Expected CallCount=1 after RecordToolCall + re-RegisterUncalledToolMetrics, got %d", snapshot2.Tools.ByTool["standalone_tool"].CallCount)
	}
}
