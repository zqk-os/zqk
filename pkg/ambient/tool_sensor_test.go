package ambient

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestToolSensor_TDD_RedPhase verifies baseline expectations for ToolExecutionRecord,
// ToolUtilizationMetrics, ToolSensor, and SCH-tool-util aggregator integration.

func TestToolSensor_RecordClassification(t *testing.T) {
	// Specialized tools: zgrep, zqk_grep, zqk read_code, etc.
	specializedTools := []string{"zgrep", "zqk grep", "zqk_grep", "zqk read_code", "zqk test run", "zqk_agent_orchestrate"}
	for _, name := range specializedTools {
		if !IsSpecializedTool(name) {
			t.Errorf("expected tool %q to be classified as specialized", name)
		}
		if IsFallbackTool(name) {
			t.Errorf("expected tool %q not to be classified as fallback", name)
		}
	}

	// Fallback tools: grep, find, cat, sed, awk, rg
	fallbackTools := []string{"grep", "find", "rg", "cat", "sed", "awk"}
	for _, name := range fallbackTools {
		if !IsFallbackTool(name) {
			t.Errorf("expected tool %q to be classified as fallback", name)
		}
		if IsSpecializedTool(name) {
			t.Errorf("expected tool %q not to be classified as specialized", name)
		}
	}
}

func TestToolSensor_RecordAndAggregate(t *testing.T) {
	sensor := NewToolSensor()

	// Record 3 specialized executions
	sensor.Record(ToolExecutionRecord{
		ToolName:    "zgrep",
		Duration:    10 * time.Millisecond,
		ExitCode:    0,
		TokensUsed:  500,
		TokensSaved: 4500,
		Success:     true,
	})
	sensor.Record(ToolExecutionRecord{
		ToolName:    "zgrep",
		Duration:    20 * time.Millisecond,
		ExitCode:    0,
		TokensUsed:  600,
		TokensSaved: 4400,
		Success:     true,
	})
	sensor.Record(ToolExecutionRecord{
		ToolName:    "zqk read_code",
		Duration:    30 * time.Millisecond,
		ExitCode:    0,
		TokensUsed:  400,
		TokensSaved: 2000,
		Success:     true,
	})

	// Record 2 fallback executions
	sensor.Record(ToolExecutionRecord{
		ToolName:    "grep",
		Duration:    100 * time.Millisecond,
		ExitCode:    0,
		TokensUsed:  5000,
		TokensSaved: 0,
		Success:     true,
	})
	sensor.Record(ToolExecutionRecord{
		ToolName:    "find",
		Duration:    200 * time.Millisecond,
		ExitCode:    1,
		TokensUsed:  3000,
		TokensSaved: 0,
		Success:     false,
	})

	metrics := sensor.Aggregate()

	if metrics.TotalInvocations != 5 {
		t.Fatalf("expected 5 total invocations, got %d", metrics.TotalInvocations)
	}
	if metrics.SpecializedInvocations != 3 {
		t.Errorf("expected 3 specialized invocations, got %d", metrics.SpecializedInvocations)
	}
	if metrics.FallbackInvocations != 2 {
		t.Errorf("expected 2 fallback invocations, got %d", metrics.FallbackInvocations)
	}
	if metrics.SuccessCount != 4 {
		t.Errorf("expected 4 successes, got %d", metrics.SuccessCount)
	}
	if metrics.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", metrics.FailureCount)
	}

	// 3 / 5 = 0.60
	expectedAdoption := 0.60
	if math.Abs(metrics.SpecializedAdoptionRate-expectedAdoption) > 1e-4 {
		t.Errorf("expected adoption rate %f, got %f", expectedAdoption, metrics.SpecializedAdoptionRate)
	}

	// 2 / 5 = 0.40
	expectedFallbackRate := 0.40
	if math.Abs(metrics.FallbackRate-expectedFallbackRate) > 1e-4 {
		t.Errorf("expected fallback rate %f, got %f", expectedFallbackRate, metrics.FallbackRate)
	}

	// 4 / 5 = 0.80
	expectedSuccessRate := 0.80
	if math.Abs(metrics.SuccessRate-expectedSuccessRate) > 1e-4 {
		t.Errorf("expected success rate %f, got %f", expectedSuccessRate, metrics.SuccessRate)
	}

	// Tokens: used = 500+600+400+5000+3000 = 9500
	if metrics.TotalTokensUsed != 9500 {
		t.Errorf("expected 9500 tokens used, got %d", metrics.TotalTokensUsed)
	}
	// Tokens saved = 4500+4400+2000 = 10900
	if metrics.TotalTokensSaved != 10900 {
		t.Errorf("expected 10900 tokens saved, got %d", metrics.TotalTokensSaved)
	}

	// Avg tokens saved per specialized = 10900 / 3 = 3633.33
	expectedAvgTokensSaved := 10900.0 / 3.0
	if math.Abs(metrics.AverageTokensSavedPerSpecializedCall-expectedAvgTokensSaved) > 1e-2 {
		t.Errorf("expected avg tokens saved %f, got %f", expectedAvgTokensSaved, metrics.AverageTokensSavedPerSpecializedCall)
	}

	// Duration: 10 + 20 + 30 + 100 + 200 = 360ms
	expectedTotalDur := 360 * time.Millisecond
	if metrics.TotalDuration != expectedTotalDur {
		t.Errorf("expected total duration %v, got %v", expectedTotalDur, metrics.TotalDuration)
	}
	expectedAvgDur := 72 * time.Millisecond
	if metrics.AverageDuration != expectedAvgDur {
		t.Errorf("expected avg duration %v, got %v", expectedAvgDur, metrics.AverageDuration)
	}

	// Per-tool breakdown check
	if len(metrics.ByTool) != 4 {
		t.Errorf("expected 4 distinct tools, got %d", len(metrics.ByTool))
	}
	zgrepStats, ok := metrics.ByTool["zgrep"]
	if !ok || zgrepStats.TotalInvocations != 2 || zgrepStats.TotalTokensSaved != 8900 {
		t.Errorf("unexpected zgrep stats: %+v", zgrepStats)
	}
}

func TestToolSensor_ZeroDivisionHandling(t *testing.T) {
	sensor := NewToolSensor()
	metrics := sensor.Aggregate()

	if metrics.TotalInvocations != 0 {
		t.Errorf("expected 0 invocations, got %d", metrics.TotalInvocations)
	}
	if metrics.SpecializedAdoptionRate != 0.0 {
		t.Errorf("expected 0.0 adoption rate, got %f", metrics.SpecializedAdoptionRate)
	}
	if metrics.FallbackRate != 0.0 {
		t.Errorf("expected 0.0 fallback rate, got %f", metrics.FallbackRate)
	}
	if metrics.SuccessRate != 0.0 {
		t.Errorf("expected 0.0 success rate, got %f", metrics.SuccessRate)
	}
	if metrics.AverageDuration != 0 {
		t.Errorf("expected 0 avg duration, got %v", metrics.AverageDuration)
	}
	if metrics.AverageTokensSavedPerSpecializedCall != 0.0 {
		t.Errorf("expected 0.0 avg tokens saved, got %f", metrics.AverageTokensSavedPerSpecializedCall)
	}

	// Formatting summary table must not panic on zero records
	summary := metrics.SummaryTable()
	if !strings.Contains(summary, "Total Invocations:") {
		t.Errorf("expected summary to contain 'Total Invocations:', got:\n%s", summary)
	}
}

func TestToolSensor_NilAndDegradedHandling(t *testing.T) {
	var nilMetrics *ToolUtilizationMetrics
	if summary := nilMetrics.SummaryTable(); summary != "<nil tool metrics>" {
		t.Errorf("expected '<nil tool metrics>', got %q", summary)
	}

	var nilSensor *ToolSensor
	if err := nilSensor.Record(ToolExecutionRecord{ToolName: "zgrep"}); err == nil {
		t.Error("expected error recording to nil sensor")
	}
	if m := nilSensor.Aggregate(); m == nil {
		t.Error("expected non-nil default metrics from nil sensor")
	}
}

func TestToolSensor_SummaryTable(t *testing.T) {
	sensor := NewToolSensor()
	sensor.Record(ToolExecutionRecord{
		ToolName:    "zgrep",
		Duration:    15 * time.Millisecond,
		ExitCode:    0,
		TokensUsed:  250,
		TokensSaved: 1750,
		Success:     true,
	})
	metrics := sensor.Aggregate()
	summary := metrics.SummaryTable()

	expectedSubstrings := []string{
		"AMBIENT TOOL UTILIZATION & EFFECTIVENESS REPORT",
		"Total Invocations:",
		"Specialized Adoption Rate:",
		"Total Tokens Saved:",
		"1750",
		"zgrep",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(summary, sub) {
			t.Errorf("summary missing expected substring %q:\n%s", sub, summary)
		}
	}
}

func TestToolSensor_ConcurrentRecording(t *testing.T) {
	sensor := NewToolSensor()
	concurrency := 20
	iterations := 100

	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		workerID := i
		goroutinelabels.NewGoroutine("tool_sensor_test", fmt.Sprintf("concurrent recording worker %d", workerID)).StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				tool := "grep"
				tokensSaved := 0
				if j%2 == 0 {
					tool = "zgrep"
					tokensSaved = 2000
				}
				err := sensor.Record(ToolExecutionRecord{
					ToolName:    tool,
					Duration:    time.Duration(j) * time.Millisecond,
					ExitCode:    0,
					TokensUsed:  100,
					TokensSaved: tokensSaved,
					Success:     true,
					AgentID:     "agent-test",
				})
				if err != nil {
					t.Errorf("unexpected record error: %v", err)
				}

				if j%25 == 0 {
					_ = sensor.Aggregate()
				}
			}
		})
	}

	wg.Wait()

	totalExpected := concurrency * iterations
	if sensor.Count() != totalExpected {
		t.Fatalf("expected count %d, got %d", totalExpected, sensor.Count())
	}

	metrics := sensor.Aggregate()
	if metrics.TotalInvocations != int64(totalExpected) {
		t.Fatalf("expected metrics total %d, got %d", totalExpected, metrics.TotalInvocations)
	}
	if metrics.SpecializedInvocations != int64(totalExpected/2) {
		t.Fatalf("expected specialized %d, got %d", totalExpected/2, metrics.SpecializedInvocations)
	}
}

func TestToolSensor_TimeWindowAggregation(t *testing.T) {
	sensor := NewToolSensor()
	now := time.Now().UTC()

	// 10 minutes ago
	sensor.Record(ToolExecutionRecord{
		ToolName:  "grep",
		Duration:  50 * time.Millisecond,
		Timestamp: now.Add(-10 * time.Minute),
		Success:   true,
	})

	// 2 minutes ago
	sensor.Record(ToolExecutionRecord{
		ToolName:  "zgrep",
		Duration:  10 * time.Millisecond,
		Timestamp: now.Add(-2 * time.Minute),
		Success:   true,
	})

	// Window: last 5 minutes
	windowMetrics := sensor.AggregateWindow(5 * time.Minute)
	if windowMetrics.TotalInvocations != 1 {
		t.Fatalf("expected 1 invocation in 5m window, got %d", windowMetrics.TotalInvocations)
	}
	if windowMetrics.SpecializedInvocations != 1 {
		t.Errorf("expected 1 specialized invocation in 5m window, got %d", windowMetrics.SpecializedInvocations)
	}

	// Full aggregation includes both
	fullMetrics := sensor.Aggregate()
	if fullMetrics.TotalInvocations != 2 {
		t.Fatalf("expected 2 total invocations in full aggregation, got %d", fullMetrics.TotalInvocations)
	}
}

func TestToolSensor_SerializationAndPersistence(t *testing.T) {
	sensor := NewToolSensor()
	sensor.Record(ToolExecutionRecord{
		ToolName:    "zgrep",
		Duration:    15 * time.Millisecond,
		ExitCode:    0,
		TokensUsed:  500,
		TokensSaved: 4000,
		Success:     true,
	})

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "tool_telemetry.json")

	err := sensor.SaveToFile(filePath)
	if err != nil {
		t.Fatalf("failed to save sensor records: %v", err)
	}

	newSensor := NewToolSensor()
	err = newSensor.LoadFromFile(filePath)
	if err != nil {
		t.Fatalf("failed to load sensor records: %v", err)
	}

	if newSensor.Count() != 1 {
		t.Fatalf("expected 1 record after load, got %d", newSensor.Count())
	}

	m := newSensor.Aggregate()
	if m.TotalTokensSaved != 4000 {
		t.Errorf("expected 4000 tokens saved, got %d", m.TotalTokensSaved)
	}

	// JSON Marshalling of metrics
	jsonData, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("failed to marshal metrics to JSON: %v", err)
	}
	var unmarshaled ToolUtilizationMetrics
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal metrics JSON: %v", err)
	}
	if unmarshaled.TotalTokensSaved != 4000 {
		t.Errorf("expected 4000 unmarshaled tokens saved, got %d", unmarshaled.TotalTokensSaved)
	}
}

func TestToolUtilizationAggregatorJob_Execution(t *testing.T) {
	ctx := context.Background()
	sensor := NewToolSensor()

	sensor.Record(ToolExecutionRecord{
		ToolName:    "zgrep",
		Duration:    20 * time.Millisecond,
		TokensUsed:  300,
		TokensSaved: 2700,
		Success:     true,
	})

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "metrics_output.json")

	// Verify the aggregator job SCH-tool-util executor hook
	metrics, err := RunToolUtilizationAggregatorJob(ctx, sensor, outPath)
	if err != nil {
		t.Fatalf("aggregator job failed: %v", err)
	}
	if metrics == nil || metrics.TotalInvocations != 1 {
		t.Fatalf("unexpected metrics from aggregator job: %+v", metrics)
	}

	// Verify file was written
	if !fileutil.Exists(outPath) {
		t.Errorf("expected output file %s to be created", outPath)
	}
}
