package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type dummyLoaderLogger struct {
	errors []string
	debugs []string
	infos  []string
}

func (d *dummyLoaderLogger) LogError(msg string, err error, fields ...logging.Field) {
	d.errors = append(d.errors, msg)
}

func (d *dummyLoaderLogger) LogDebug(msg string, fields ...logging.Field) {
	d.debugs = append(d.debugs, msg)
}

func (d *dummyLoaderLogger) LogInfo(msg string, fields ...logging.Field) {
	d.infos = append(d.infos, msg)
}

func (d *dummyLoaderLogger) LogWarning(msg string, fields ...logging.Field) {
	d.infos = append(d.infos, msg)
}

func TestParagraphBuilder_Comprehensive(t *testing.T) {
	pb := NewParagraphBuilder()
	pb.AddLine("Line 1").
		AddLinef("Line %d", 2).
		AddLineWithIndent(2, "Indented Line 3").
		AddLineWithIndentf(4, "Indented Line %d", 4).
		Add("Chunk A ").
		Add("Chunk B").
		AddIf(true, " Condition True").
		AddIf(false, " Condition False").
		BlankLine()

	out := pb.Build()
	if !strings.Contains(out, "Line 1\n") {
		t.Errorf("missing Line 1 in %q", out)
	}
	if !strings.Contains(out, "Line 2\n") {
		t.Errorf("missing Line 2 in %q", out)
	}
	if !strings.Contains(out, "  Indented Line 3\n") {
		t.Errorf("missing indented line 3 in %q", out)
	}
	if !strings.Contains(out, "    Indented Line 4\n") {
		t.Errorf("missing indented line 4 in %q", out)
	}
	if !strings.Contains(out, "Chunk A Chunk B Condition True\n") {
		t.Errorf("missing chunk condition line in %q", out)
	}
	if strings.Contains(out, "Condition False") {
		t.Errorf("unexpected Condition False in %q", out)
	}
}

func TestStreamTable_Comprehensive(t *testing.T) {
	var buf bytes.Buffer
	title := "Stream Table Title"
	columns := []string{"Col1", "Col2"}
	widths := []int{10, 15}

	rowChan := make(chan []string, 2)
	rowChan <- []string{"val1", "val2"}
	rowChan <- []string{"val3-long-string-that-needs-truncation", "val4"}
	close(rowChan)

	err := StreamTable(&buf, title, columns, widths, rowChan)
	if err != nil {
		t.Fatalf("StreamTable failed: %v", err)
	}

	res := buf.String()
	if !strings.Contains(res, title) {
		t.Errorf("missing title in stream output: %q", res)
	}
	if !strings.Contains(res, "COL1") || !strings.Contains(res, "COL2") {
		t.Errorf("missing column headers: %q", res)
	}
	if !strings.Contains(res, "val1") {
		t.Errorf("missing val1: %q", res)
	}
}

func TestRenderTableWithTitleAndWrap(t *testing.T) {
	layer4Rows := [][]string{
		{"Open File Descriptors", "9 / 245760", "\x1b[32m✓ healthy\x1b[0m"},
		{".zqk Storage Volume", "26559 files (208MiB)", "\x1b[36m✓ tracked\x1b[0m"},
		{"Stale Lock Files", "1", "\x1b[33m⚠️ attention (1 stale .lock files)\x1b[0m"},
		{"Orphaned Temp Files", "0", "\x1b[32m✓ clean (0 orphaned)\x1b[0m"},
	}

	out := RenderTableWithTitleAndWrap("Layer 4: I/O Resource Hygiene & Storage Telemetry", []string{"METRIC", "VALUE", "STATUS"}, []int{30, 20, 40}, layer4Rows)
	lines := strings.Split(out, "\n")
	if len(lines) == 0 {
		t.Fatal("expected rendered table lines, got empty")
	}

	for i, line := range lines {
		if strings.HasPrefix(line, "│") && !strings.HasSuffix(line, "│") {
			t.Errorf("line %d does not end with closing border: %q", i, line)
		}
	}
}

func TestQueryFlags_Comprehensive(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	AddQueryFlags(cmd)

	// Test default / empty parsing
	qf, err := ParseQueryFlags(cmd, nil, func(s string) (string, any, error) {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) == 2 {
			return parts[0], parts[1], nil
		}
		return s, true, nil
	})
	if err != nil {
		t.Fatalf("ParseQueryFlags failed: %v", err)
	}
	if qf == nil {
		t.Fatal("expected non-nil QueryFlags")
	}

	// Test BuildListFilterData & calculateEffectiveLimit
	qf.Limit = 50
	qf.Offset = 10
	qf.SortBy = "name"
	qf.SortAsc = false
	qf.GroupBy = "status"
	qf.GroupLimit = 5
	qf.Fields = []string{"id", "title"}
	qf.Filters["status"] = "open"

	filterData := BuildListFilterData("backlog_item", qf, 100)
	if filterData["kind"] != "backlog_item" {
		t.Errorf("expected kind=backlog_item, got %v", filterData["kind"])
	}
	if filterData["limit"] != 50 {
		t.Errorf("expected limit=50, got %v", filterData["limit"])
	}

	qf.Limit = -1
	filterDataZero := BuildListFilterData("task", qf, 100)
	if filterDataZero["limit"] != 0 {
		t.Errorf("expected limit=0 for negative limit, got %v", filterDataZero["limit"])
	}

	// Test AddCountFlags & ParseCountFlags & CountHarnessFlagNames
	countCmd := &cobra.Command{Use: "count"}
	AddCountFlags(countCmd)

	harnessNames := CountHarnessFlagNames()
	if len(harnessNames) != 3 {
		t.Errorf("expected 3 count harness flag names, got %d", len(harnessNames))
	}

	filters, groupBy, err := ParseCountFlags(countCmd, nil, func(s string) (string, any, error) {
		return s, "val", nil
	})
	if err != nil {
		t.Fatalf("ParseCountFlags failed: %v", err)
	}
	if groupBy != "" {
		t.Errorf("expected empty groupBy, got %q", groupBy)
	}
	if len(filters) != 0 {
		t.Errorf("expected empty filters, got %v", filters)
	}
}

func TestUpdateDataLoader_Comprehensive(t *testing.T) {
	// ParseFieldFlag tests
	name, val, err := ParseFieldFlag("title=New Title", nil)
	if err != nil || name != "title" || val != "New Title" {
		t.Errorf("ParseFieldFlag with = failed: name=%q, val=%q, err=%v", name, val, err)
	}

	name, val, err = ParseFieldFlag("status:completed", nil)
	if err != nil || name != "status" || val != "completed" {
		t.Errorf("ParseFieldFlag with : failed: name=%q, val=%q, err=%v", name, val, err)
	}

	_, _, err = ParseFieldFlag("invalidformat", nil)
	if err == nil {
		t.Error("expected error for invalid format, got nil")
	}

	// ParseFieldValue tests
	v1 := ParseFieldValue("")
	if v1 != "" {
		t.Errorf("expected empty string, got %v", v1)
	}
	v2 := ParseFieldValue("123")
	if v2 != float64(123) {
		t.Errorf("expected float64(123), got %v", v2)
	}
	v3 := ParseFieldValue(`{"key": "val"}`)
	if m, ok := v3.(map[string]any); !ok || m["key"] != "val" {
		t.Errorf("expected parsed JSON map, got %v", v3)
	}
	v4 := ParseFieldValue("just a plain string")
	if v4 != "just a plain string" {
		t.Errorf("expected plain string, got %v", v4)
	}

	// BuildUpdatesFromFieldFlags
	cmd := &cobra.Command{Use: "update"}
	cmd.Flags().StringArray("field", []string{"name=Alice", "age=30"}, "fields")
	updates, err := BuildUpdatesFromFieldFlags(cmd, nil)
	if err != nil {
		t.Fatalf("BuildUpdatesFromFieldFlags failed: %v", err)
	}
	if updates["name"] != "Alice" || updates["age"] != float64(30) {
		t.Errorf("unexpected updates: %v", updates)
	}

	// LoadUpdatesFromData
	dataCmd := &cobra.Command{Use: "update"}
	dataCmd.Flags().String("data", "owner: Bob\nscore: 100", "data")
	dataUpdates, err := LoadUpdatesFromData(dataCmd, nil)
	if err != nil {
		t.Fatalf("LoadUpdatesFromData failed: %v", err)
	}
	if dataUpdates["owner"] != "Bob" || dataUpdates["score"] != 100 {
		t.Errorf("unexpected dataUpdates: %v", dataUpdates)
	}

	// LoadUpdatesFromFile
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "update.yaml")
	if err := fileutil.WriteFile(filePath, []byte("priority: high\nactive: true\n"), fileutil.StandardFilePerm); err != nil {
		t.Fatalf("failed to write tmp file: %v", err)
	}
	fileCmd := &cobra.Command{Use: "update"}
	fileCmd.Flags().String("file", filePath, "file")
	fileUpdates, err := LoadUpdatesFromFile(fileCmd, nil)
	if err != nil {
		t.Fatalf("LoadUpdatesFromFile failed: %v", err)
	}
	if fileUpdates["priority"] != "high" || fileUpdates["active"] != true {
		t.Errorf("unexpected fileUpdates: %v", fileUpdates)
	}

	// BuildUpdatesMap
	fullCmd := &cobra.Command{Use: "update"}
	fullCmd.Flags().StringArray("field", []string{"foo=bar"}, "fields")
	fullCmd.Flags().String("file", "", "file")
	fullCmd.Flags().String("data", "", "data")
	allUpdates, err := BuildUpdatesMap(fullCmd, nil)
	if err != nil {
		t.Fatalf("BuildUpdatesMap failed: %v", err)
	}
	if allUpdates["foo"] != "bar" {
		t.Errorf("expected foo=bar, got %v", allUpdates)
	}

	// Empty updates map returns error
	emptyCmd := &cobra.Command{Use: "update"}
	emptyCmd.Flags().StringArray("field", []string{}, "fields")
	emptyCmd.Flags().String("file", "", "file")
	emptyCmd.Flags().String("data", "", "data")
	_, err = BuildUpdatesMap(emptyCmd, nil)
	if err == nil {
		t.Error("expected error for empty updates, got nil")
	}
}

func TestMetricsAnalyzer_ReportOutput(t *testing.T) {
	analyzer := NewMetricsAnalyzer(nil)

	res := &AnalysisResult{
		HighFailureRateCommands: []CommandIssue{
			{NormalizedCmd: "test fail", Issue: "Failure rate 80%", Severity: "high"},
		},
		FrequentTimeouts: []CommandIssue{
			{NormalizedCmd: "test timeout", Issue: "Timeout rate 50%", Severity: "high"},
		},
		SlowCommands: []CommandIssue{
			{NormalizedCmd: "test slow", Issue: "Average duration 2s", Severity: "medium"},
		},
		ImprovementSuggestions: []Suggestion{
			{Command: "test fail", Description: "fix bugs", Priority: "high", Impact: "high"},
		},
		ChurnIndicators: []ChurnIndicator{
			{Pattern: "high_retry_rate", Description: "many retries", Severity: "medium", Commands: []string{"test fail"}},
		},
	}

	report := analyzer.GenerateReport(res)
	if !strings.Contains(report, "Command Metrics Analysis Report") {
		t.Errorf("unexpected report header: %q", report)
	}

	jsonBytes, err := analyzer.GenerateReportJSON(res)
	if err != nil {
		t.Fatalf("GenerateReportJSON failed: %v", err)
	}
	if !strings.Contains(string(jsonBytes), "high_failure_rate_commands") {
		t.Errorf("unexpected json: %s", string(jsonBytes))
	}

	yamlBytes, err := analyzer.GenerateReportYAML(res)
	if err != nil {
		t.Fatalf("GenerateReportYAML failed: %v", err)
	}
	if !strings.Contains(string(yamlBytes), "high_failure_rate_commands:") {
		t.Errorf("unexpected yaml: %s", string(yamlBytes))
	}

	// Nil analysis result payload test
	nilPayload := analyzer.AnalysisReportPayload(nil)
	if nilPayload["generated_at"] == nil {
		t.Error("expected generated_at in nil payload")
	}

	// Helper functions
	if getBaseCommand("zqk system check") != "zqk" {
		t.Errorf("expected zqk, got %q", getBaseCommand("zqk system check"))
	}
	if getBaseCommand("single") != "single" {
		t.Errorf("expected single, got %q", getBaseCommand("single"))
	}
	cmdList := []*CommandMetrics{{NormalizedCmd: "a"}, {NormalizedCmd: "b"}}
	names := getCommandNames(cmdList)
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Errorf("unexpected command names: %v", names)
	}
}

func TestTimeoutHook_Comprehensive(t *testing.T) {
	hook := NewTimeoutHook()
	hook.SetEnabled(true)
	hook.SetMaxTimeout(10 * time.Minute)
	hook.SetCommandTimeout("custom cmd", 30*time.Second)

	// Normalization tests
	norm := hook.normalizeCommand("zqk object get", []string{"BLI-1234", "--detail"})
	if !strings.Contains(norm, "<arg>") || !strings.Contains(norm, "--detail") {
		t.Errorf("unexpected normalized command: %q", norm)
	}

	uuidArg := "12345678-1234-1234-1234-123456789012"
	if !hook.isVariableArg(uuidArg) {
		t.Errorf("expected uuid %q to be variable arg", uuidArg)
	}
	if !hook.isVariableArg("/path/to/file") {
		t.Error("expected path to be variable arg")
	}
	if !hook.isVariableArg("./rel/file") {
		t.Error("expected rel path to be variable arg")
	}
	if !hook.isVariableArg("~/home/file") {
		t.Error("expected home path to be variable arg")
	}

	sanitized := hook.sanitizeArgs([]string{"BLI-1234", "flag"})
	if sanitized[0] != "<sanitized>" || sanitized[1] != "flag" {
		t.Errorf("unexpected sanitized args: %v", sanitized)
	}

	errMsg := hook.sanitizeError(os.ErrNotExist)
	if errMsg == "" {
		t.Error("expected non-empty sanitized error")
	}
	if hook.sanitizeError(nil) != "" {
		t.Error("expected empty string for nil error")
	}

	// Resettable server context
	ctx := context.Background()
	setServerContext(ctx)
	if GetServerContext() != ctx {
		t.Errorf("expected ctx %v, got %v", ctx, GetServerContext())
	}
	calledReset := false
	setTimeoutResetFunc(func() {
		calledReset = true
	})
	ResetTimeout()
	if !calledReset {
		t.Error("expected timeout reset function to be invoked")
	}

	// Timeout flag parsing & override
	d1, expl1 := hook.parseTimeoutFlag(&CommandContext{
		Flags: map[string]any{"timeout": "15s"},
	}, nil)
	if !expl1 || d1 != 15*time.Second {
		t.Errorf("expected 15s explicitly set, got %v (%v)", d1, expl1)
	}

	d2, expl2 := hook.parseTimeoutFlag(nil, []string{"--timeout", "45s"})
	if !expl2 || d2 != 45*time.Second {
		t.Errorf("expected 45s explicitly set, got %v (%v)", d2, expl2)
	}

	overrideD := hook.applyUserTimeoutOverride(10*time.Second, 20*time.Second, true, "some cmd")
	if overrideD != 20*time.Second {
		t.Errorf("expected 20s, got %v", overrideD)
	}

	overrideSysCheck := hook.applyUserTimeoutOverride(10*time.Second, 30*time.Second, true, "system check --auto-fix")
	if overrideSysCheck < 2*time.Minute {
		t.Errorf("expected >= 2m for system check auto fix, got %v", overrideSysCheck)
	}

	overrideDisabled := hook.applyUserTimeoutOverride(10*time.Second, 0, true, "some cmd")
	if overrideDisabled != 0 {
		t.Errorf("expected 0 (disabled), got %v", overrideDisabled)
	}

	// estimateObjectCountForSystemCheck
	count1 := hook.estimateObjectCountForSystemCheck([]string{"BLI-1234", "AUD-5678"})
	if count1 != 2 {
		t.Errorf("expected count 2, got %d", count1)
	}
	count2 := hook.estimateObjectCountForSystemCheck([]string{"backlog_item"})
	if count2 != 1000 {
		t.Errorf("expected count 1000, got %d", count2)
	}
	count3 := hook.estimateObjectCountForSystemCheck([]string{"all"})
	if count3 != 5000 {
		t.Errorf("expected count 5000, got %d", count3)
	}

	// WrapCommand simple execution
	executed := false
	err := hook.WrapCommand(context.Background(), "test cmd", []string{}, func(execCtx context.Context) error {
		executed = true
		return nil
	})
	if err != nil || !executed {
		t.Fatalf("WrapCommand failed: err=%v, executed=%v", err, executed)
	}
}

func TestDisplayFields_Comprehensive(t *testing.T) {
	spec := &objects.Spec{
		ResolvedFields: map[string]any{
			"summary": map[string]any{
				"validation": map[string]any{
					"display_length": 10,
				},
			},
			"description": map[string]any{
				"validation": map[string]any{
					"display_length": 5,
				},
			},
		},
	}

	obj := map[string]any{
		"summary":     "This is a very long summary string",
		"description": "Short",
		"number":      42,
	}

	AddDisplayFieldsForTable(obj, spec, nil)

	if obj["d_summary"] == nil {
		t.Error("expected d_summary to be created")
	}
	if obj["d_description"] != nil {
		t.Error("expected d_description to be omitted when length <= display_length")
	}

	// GetDisplayValue
	val1 := GetDisplayValue(obj, "summary", false)
	if val1 != obj["d_summary"] {
		t.Errorf("expected d_summary, got %v", val1)
	}

	val2 := GetDisplayValue(obj, "summary", true)
	if val2 != obj["d_summary"] {
		t.Errorf("expected d_summary for non-never-truncate field in detail view, got %v", val2)
	}

	val3 := GetDisplayValue(obj, "description", true)
	if val3 != "Short" {
		t.Errorf("expected Short for description in detail view, got %v", val3)
	}

	// formatValue edge cases
	now := time.Now()
	if formatValue(nil) != "" {
		t.Error("expected empty string for nil")
	}
	if formatValue(now) != now.Format(time.RFC3339) {
		t.Error("expected RFC3339 time string")
	}
	if formatValue(&now) != now.Format(time.RFC3339) {
		t.Error("expected RFC3339 time pointer string")
	}
	var nilTime *time.Time
	if formatValue(nilTime) != "" {
		t.Error("expected empty string for nil *time.Time")
	}
}

func TestBulkOutput_Comprehensive(t *testing.T) {
	results := []map[string]any{
		{"id": "BLI-1", "status": "ok"},
	}
	errors := []BulkErrorInfo{
		{ID: "BLI-2", Index: 1, Message: "failed to update"},
	}

	data := BuildBulkResultData(2, 1, 1, results, errors, "update")
	if data["operation"] != "update" || data["total"] != 2 {
		t.Errorf("unexpected bulk data: %v", data)
	}

	jsonBytes, err := OutputBulkResult(2, 1, 1, results, errors, "update", "json")
	if err != nil || !strings.Contains(string(jsonBytes), `"operation": "update"`) {
		t.Fatalf("OutputBulkResult JSON failed: %v", err)
	}

	yamlBytes, err := OutputBulkResult(2, 1, 1, results, errors, "update", "yaml")
	if err != nil || !strings.Contains(string(yamlBytes), "operation: update") {
		t.Fatalf("OutputBulkResult YAML failed: %v", err)
	}

	tableBytes, err := OutputBulkResult(2, 1, 1, results, errors, "update", "table")
	if err != nil || !strings.Contains(string(tableBytes), "Bulk update operation completed:") {
		t.Fatalf("OutputBulkResult Table failed: %v", err)
	}

	defaultBytes, err := OutputBulkResult(2, 1, 1, results, errors, "update", "unknown_format")
	if err != nil || !strings.Contains(string(defaultBytes), "Bulk update operation completed:") {
		t.Fatalf("OutputBulkResult default format failed: %v", err)
	}
}

func TestCountOutput_Comprehensive(t *testing.T) {
	res := &CountResult{
		Objects: []map[string]any{{"id": "1"}, {"id": "2"}},
		Groups: map[string][]map[string]any{
			"active":   {{"id": "1"}},
			"inactive": {{"id": "2"}},
		},
		Meta: map[string]any{
			"total_count":    2,
			"returned_count": 2,
			"total_groups":   2,
		},
	}

	jsonBytes, err := OutputCount(res, "json", "task", "status")
	if err != nil || !strings.Contains(string(jsonBytes), "counts_by_group") {
		t.Fatalf("OutputCount JSON grouped failed: %v", err)
	}

	yamlBytes, err := OutputCount(res, "yaml", "task", "status")
	if err != nil || !strings.Contains(string(yamlBytes), "counts_by_group:") {
		t.Fatalf("OutputCount YAML grouped failed: %v", err)
	}

	tableBytes, err := OutputCount(res, "table", "task", "status")
	if err != nil || !strings.Contains(string(tableBytes), "STATUS") {
		t.Fatalf("OutputCount Table grouped failed: %v", err)
	}

	// Ungrouped
	ungrouped := &CountResult{
		Objects: []map[string]any{{"id": "1"}},
		Meta: map[string]any{
			"total_count":    1,
			"returned_count": 1,
		},
	}
	jsonBytes, err = OutputCount(ungrouped, "json", "task", "")
	if err != nil || !strings.Contains(string(jsonBytes), `"count": 1`) {
		t.Fatalf("OutputCount JSON ungrouped failed: %v", err)
	}
	yamlBytes, err = OutputCount(ungrouped, "yaml", "task", "")
	if err != nil || !strings.Contains(string(yamlBytes), "count: 1") {
		t.Fatalf("OutputCount YAML ungrouped failed: %v", err)
	}
	tableBytes, err = OutputCount(ungrouped, "table", "task", "")
	if err != nil || !strings.Contains(string(tableBytes), "task") {
		t.Fatalf("OutputCount Table ungrouped failed: %v", err)
	}

	directJSON, err := OutputCountDirect(5, "json", "task", map[string]any{"namespace": "default"})
	if err != nil || !strings.Contains(string(directJSON), `"count": 5`) {
		t.Fatalf("OutputCountDirect JSON failed: %v", err)
	}
}

func TestDryRun_Comprehensive(t *testing.T) {
	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().Bool("dry-run", false, "dry run")
	logger := &dummyLoaderLogger{}

	// Not set
	handled, _, err := HandleDryRun(cmd, map[string]any{"id": "1"}, "task", logger, "task")
	if err != nil || handled {
		t.Fatalf("expected handled=false, got %v, err=%v", handled, err)
	}

	// Set true
	_ = cmd.Flags().Set("dry-run", "true")
	handled, result, err := HandleDryRun(cmd, map[string]any{"id": "1"}, "task", logger, "task")
	if err != nil || !handled || result == nil {
		t.Fatalf("expected handled=true, got %v, err=%v", handled, err)
	}
	if result.Message != "Would create task" {
		t.Errorf("unexpected message: %q", result.Message)
	}

	// HandleDeleteDryRun
	delCmd := &cobra.Command{Use: "delete"}
	delCmd.Flags().Bool("dry-run", true, "dry run")
	handled, delRes, err := HandleDeleteDryRun(delCmd, "ID-1", map[string]any{"id": "ID-1"}, true, logger, "custom_kind")
	if err != nil || !handled || delRes == nil {
		t.Fatalf("expected handled=true for delete dry run, got %v, err=%v", handled, err)
	}
	if delRes.Message != "Would delete custom_kind" || !delRes.Cascade {
		t.Errorf("unexpected delete dry run result: %v", delRes)
	}

	// HandleUpdateDryRun
	upCmd := &cobra.Command{Use: "update"}
	upCmd.Flags().Bool("dry-run", true, "dry run")
	handled, upRes, err := HandleUpdateDryRun(upCmd, "ID-1", map[string]any{"title": "Old"}, map[string]any{"title": "New", "tag": "v1"}, false, logger, "task")
	if err != nil || !handled || upRes == nil {
		t.Fatalf("expected handled=true for update dry run, got %v, err=%v", handled, err)
	}
	if upRes.Message != "Would update task" {
		t.Errorf("unexpected update dry run message: %q", upRes.Message)
	}
	if upRes.Changed["title"].Old != "Old" || upRes.Changed["title"].New != "New" {
		t.Errorf("unexpected change for title: %v", upRes.Changed["title"])
	}
	if upRes.Changed["tag"].Old != nil || upRes.Changed["tag"].New != "v1" {
		t.Errorf("unexpected change for tag: %v", upRes.Changed["tag"])
	}
}

func TestAliasRegistry_AdditionalMethods(t *testing.T) {
	ar := NewAliasRegistry()
	ar.Register("custom_canonical", []string{"custom_alias"})

	if ar.ResolveToSingle("custom_alias") != "custom_canonical" {
		t.Errorf("expected custom_canonical, got %s", ar.ResolveToSingle("custom_alias"))
	}
	if ar.ResolveToSingle("unknown") != "unknown" {
		t.Errorf("expected unknown, got %s", ar.ResolveToSingle("unknown"))
	}

	aliases := ar.GetAliases("custom_canonical")
	if len(aliases) != 1 || aliases[0] != "custom_alias" {
		t.Errorf("expected ['custom_alias'], got %v", aliases)
	}
	if len(ar.GetAliases("unknown")) != 0 {
		t.Errorf("expected empty aliases for unknown")
	}

	if !ar.HasAlias("custom_alias") {
		t.Error("expected HasAlias('custom_alias') == true")
	}
	if !ar.HasAlias("custom_canonical") {
		t.Error("expected HasAlias('custom_canonical') == true")
	}
	if ar.HasAlias("something_else") {
		t.Error("expected HasAlias('something_else') == false")
	}
}

func TestCommandBuilder_AdditionalMethods(t *testing.T) {
	cb := NewCommandBuilder("sample").
		WithUse("sample [args]").
		WithHidden(true).
		WithVersion("1.0.0").
		WithGroupID("core").
		WithAnnotations(map[string]string{"foo": "bar"}).
		WithValidArgsFunction(func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{"opt1"}, cobra.ShellCompDirectiveNoFileComp
		}).
		WithCommonFlagsExcluding(func(cmd *cobra.Command, exclude []string) {
			cmd.Flags().Bool("custom-common", true, "custom")
		}, []string{"format"}).
		WithCommonFlagsDefaultExcluding(func(cmd *cobra.Command, exclude []string) {
			cmd.Flags().Bool("default-excluded", true, "default")
		}).
		WithCommonFlagsDefaultExcludingWithout(func(cmd *cobra.Command, exclude []string) {
			cmd.Flags().Bool("without-excluded", true, "without")
		}, "format").
		AddStringFlag("str", "s", "def", "str flag").
		AddStringArrayFlag("str-arr", "a", "str array").
		AddDurationFlag("dur", "d", "5s", "duration flag").
		WithHelpFunc(func(cmd *cobra.Command, args []string) {}).
		WithEntitlementBundle("bundle.admin").
		WithQueryFlags()

	cmd := cb.Build()
	if cmd.Use != "sample [args]" {
		t.Errorf("expected use 'sample [args]', got %q", cmd.Use)
	}
	if !cmd.Hidden {
		t.Error("expected hidden command")
	}
	if cmd.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", cmd.Version)
	}
	if cmd.GroupID != "core" {
		t.Errorf("expected group ID 'core', got %s", cmd.GroupID)
	}
	if cmd.Annotations["foo"] != "bar" {
		t.Errorf("expected annotation foo=bar, got %v", cmd.Annotations)
	}

	// CRUDCommandBuilder methods
	crud := NewCRUDCommandBuilder("create", "object").
		WithCommonFlagsExcluding(func(cmd *cobra.Command, exclude []string) {}, []string{"format"}).
		WithDataInputFlags().
		WithUpdateFlags().
		WithUnlinkReferencesFlag().
		WithQueryFlags()

	crudCmd := crud.Build()
	if crudCmd.Flags().Lookup("unlink-references") == nil {
		t.Error("expected unlink-references flag")
	}
	if crudCmd.Flags().Lookup("auto-status") == nil {
		t.Error("expected auto-status flag")
	}

	// ApplyBuilder
	base := &cobra.Command{Use: "base"}
	override := &cobra.Command{
		Use:        "overridden",
		Short:      "short desc",
		Long:       "long desc",
		Example:    "example usage",
		Hidden:     true,
		Aliases:    []string{"alias1"},
		Deprecated: "use other",
	}
	merged := ApplyBuilder(base, override)
	if merged.Use != "overridden" || merged.Short != "short desc" || !merged.Hidden || merged.Deprecated != "use other" {
		t.Errorf("ApplyBuilder failed to apply overrides: %v", merged)
	}
}

func TestCRUDHelpers_Comprehensive(t *testing.T) {
	logger := &dummyLoaderLogger{}

	// EnsureKindMatches
	data := map[string]any{}
	if err := EnsureKindMatches(data, "task", logger); err != nil || data["kind"] != "task" {
		t.Errorf("EnsureKindMatches failed to set kind: err=%v, data=%v", err, data)
	}

	if err := EnsureKindMatches(data, "other", logger); err == nil {
		t.Error("expected error on kind mismatch")
	}

	// isTempFilePath & CleanupSourceFile
	if !isTempFilePath("/tmp/tmp-test.yaml") || !isTempFilePath("tmp_test.yaml") {
		t.Error("expected tmp file paths to be recognized")
	}
	if isTempFilePath("/path/to/normal.yaml") || isTempFilePath("") {
		t.Error("expected normal paths to not be temp files")
	}

	tmpDir := t.TempDir()
	tempFile := filepath.Join(tmpDir, "tmp-cleanup.yaml")
	_ = fileutil.WriteFile(tempFile, []byte("test"), fileutil.StandardFilePerm)

	cleanCmd := &cobra.Command{Use: "test"}
	cleanCmd.Flags().Bool("keep-file", false, "keep")
	CleanupSourceFile(cleanCmd, tempFile, logger)
	if fileutil.Exists(tempFile) {
		t.Error("expected temp file to be cleaned up")
	}

	// Format messages
	msg1 := FormatCreateSuccessMessage(map[string]any{"id": "BLI-1"}, "backlog_item", "Backlog item", logger)
	if !strings.Contains(msg1, "BLI-1") {
		t.Errorf("expected BLI-1 in message: %s", msg1)
	}

	msg2 := FormatCreateSuccessMessage(map[string]any{}, "backlog_item", "Backlog item", logger)
	if !strings.Contains(msg2, "created successfully") {
		t.Errorf("expected created successfully in message: %s", msg2)
	}

	msg3 := FormatDeleteSuccessMessage("ID-1", true, true, "Item", logger)
	if !strings.Contains(msg3, "Built-in") || !strings.Contains(msg3, "dependents") {
		t.Errorf("unexpected delete msg: %s", msg3)
	}

	msg4 := FormatDeleteSuccessMessage("ID-2", false, false, "Item", logger)
	if !strings.Contains(msg4, "ID-2") {
		t.Errorf("unexpected delete msg: %s", msg4)
	}

	msg5 := FormatUpdateSuccessMessage("ID-1", true, "Item", logger)
	if !strings.Contains(msg5, "Built-in") {
		t.Errorf("unexpected update msg: %s", msg5)
	}

	msg6 := FormatUpdateSuccessMessage("ID-2", false, "Item", logger)
	if !strings.Contains(msg6, "ID-2") {
		t.Errorf("unexpected update msg: %s", msg6)
	}
}

func TestDataLoader_And_IDLoader(t *testing.T) {
	logger := &dummyLoaderLogger{}

	// LoadObjectDataFromString
	data, _, err := LoadObjectDataFromString("title: Test Title\npriority: 1", logger)
	if err != nil || data["title"] != "Test Title" {
		t.Fatalf("LoadObjectDataFromString failed: err=%v, data=%v", err, data)
	}

	// LoadObjectData with inline --data
	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("file", "", "file")
	cmd.Flags().String("data", "owner: Bob", "data")
	cmd.Flags().StringArray("field", []string{"tag=urgent"}, "field")
	loaded, _, err := LoadObjectData(cmd, logger)
	if err != nil || loaded["owner"] != "Bob" || loaded["tag"] != "urgent" {
		t.Fatalf("LoadObjectData failed: err=%v, loaded=%v", err, loaded)
	}

	// LoadIDsFromFlags
	idCmd := &cobra.Command{Use: "get"}
	idCmd.Flags().String("ids", "BLI-1,BLI-2", "ids")
	idCmd.Flags().String("file", "", "file")
	ids, err := LoadIDsFromFlags(idCmd, logger)
	if err != nil || len(ids) != 2 || ids[0] != "BLI-1" {
		t.Fatalf("LoadIDsFromFlags failed: err=%v, ids=%v", err, ids)
	}
}

func TestFlagBag_AdditionalTypes(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringSlice("slice", []string{"a", "b"}, "slice")
	cmd.Flags().Duration("dur", 10*time.Second, "dur")
	cmd.Flags().Float64("flt", 3.14, "flt")
	cmd.Flags().StringToString("map", map[string]string{"k": "v"}, "map")

	var bag FlagBag
	s := bag.StringSlice(cmd, "slice")
	d := bag.Duration(cmd, "dur")
	f := bag.Float64(cmd, "flt")
	m := bag.StringToString(cmd, "map")

	if len(s) != 2 || s[0] != "a" {
		t.Errorf("unexpected slice: %v", s)
	}
	if d != 10*time.Second {
		t.Errorf("unexpected duration: %v", d)
	}
	if f != 3.14 {
		t.Errorf("unexpected float: %v", f)
	}
	if m["k"] != "v" {
		t.Errorf("unexpected map: %v", m)
	}
}

func TestContextHUD_Comprehensive(t *testing.T) {
	// Nil object
	EmitContextHUD(context.Background(), nil, nil, "updating", "ID-1")

	// Local namespace (default kernel) -> skipped
	objKernel := map[string]any{"namespace_id": "zqk:kernel"}
	EmitContextHUD(context.Background(), nil, objKernel, "updating", "ID-1")

	// Non-local namespace with Human actor
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	objMesh := map[string]any{"namespace_id": "zqk:mesh"}
	secCtxUser := &pkgctx.SecurityContext{AccountID: "ACC-USER-1"}
	EmitContextHUD(ctx, secCtxUser, objMesh, "deleting", "ID-2")
	if !strings.Contains(buf.String(), "Mesh Context: zqk:mesh") || !strings.Contains(buf.String(), "User 'ACC-USER-1'") {
		t.Errorf("unexpected HUD output: %q", buf.String())
	}

	// Agent actor
	buf.Reset()
	secCtxAgent := &pkgctx.SecurityContext{AccountID: "agent:coder"}
	EmitContextHUD(ctx, secCtxAgent, objMesh, "creating", "ID-3")
	if !strings.Contains(buf.String(), "Agent 'agent:coder'") {
		t.Errorf("unexpected agent HUD output: %q", buf.String())
	}
}

func TestHelpBuilder_Comprehensive(t *testing.T) {
	hb := NewHelpBuilder().WithShort("Test short description")
	hb.AddDescriptionLine("Line 1").
		AddDescriptionLine("Line 2").
		AddDescriptionParagraph("Paragraph 1").
		AddDescriptionParagraph("Paragraph 2").
		AddExample("Example comment", "zqk test-cmd --flag").
		AddSection("Notes", "Some important notes here").
		WithAutoDiscoverFlags(true).
		WithAutoDiscoverSubcommands(true).
		WithIncludeCommonFlags(true).
		ExcludeFlag("excluded-flag").
		ExcludeFlags("ex1", "ex2").
		ExcludeCommonFlags().
		ExcludeCommonFlagsWithout("format").
		CategorizeFlag("Options", "custom-opt").
		WithSpecExamples("backlog_item").
		WithTerminalWidth(80)

	cmd := &cobra.Command{Use: "test-cmd"}
	hb.ApplyToCommand(cmd)
	if cmd.Short != "Test short description" {
		t.Errorf("expected cmd.Short to be 'Test short description', got %q", cmd.Short)
	}

	out := hb.Build()
	if !strings.Contains(out, "Line 1") || !strings.Contains(out, "Paragraph 1") {
		t.Errorf("missing description content: %q", out)
	}
	if !strings.Contains(out, "Notes:") {
		t.Errorf("missing section in help output: %q", out)
	}
}

type queryableMockGraphConnection struct {
	mockGraphConnection
	queryResult *provider.QueryResult
	queryErr    error
}

func (m *queryableMockGraphConnection) ExecuteQuery(ctx context.Context, query provider.Query) (*provider.QueryResult, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	if m.queryResult != nil {
		return m.queryResult, nil
	}
	return m.mockGraphConnection.ExecuteQuery(ctx, query)
}

func TestSemanticTraverser_Comprehensive(t *testing.T) {
	ctx := context.Background()

	// Conn is nil
	nilST := NewSemanticTraverser(nil)
	if _, err := nilST.FindCommands(ctx, "find tasks"); err == nil {
		t.Error("expected error for nil connection in FindCommands")
	}

	// Mock connection
	node1 := &provider.Node{
		ID:     "node-1",
		Labels: []string{"CommandSpec"},
		Properties: map[string]any{
			objects.FieldKeyName:       "task-create",
			objects.FieldKeyType:       "cli",
			"operation_type":           "create",
			objects.FieldKeyTargetKind: "backlog_item",
			objects.FieldKeyFilePath:   "/path/to/spec.yaml",
		},
	}
	conn := &queryableMockGraphConnection{
		queryResult: &provider.QueryResult{
			Nodes: []*provider.Node{node1},
		},
	}

	reg := NewAliasRegistry()
	reg.Register("backlog_item", []string{"tasks", "item_synonym"})
	reg.Register("agent_task", []string{"item_synonym"})
	st := NewSemanticTraverser(conn).WithAliasRegistry(reg)

	// Query with multiple target kinds (IN clause)
	_, _ = st.FindCommands(ctx, "create for item_synonym")
	_, _ = st.TraverseCommands(ctx).For("item_synonym").Execute()

	// Query with Nodes
	results, err := st.FindCommands(ctx, "create tasks for backlog_item")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Name != "task-create" {
		t.Errorf("unexpected results: %+v", results)
	}

	// Query with Rows fallback
	conn.queryResult = &provider.QueryResult{
		Rows: []map[string]any{
			{"cmd": node1},
		},
	}
	results, err = st.FindCommands(ctx, "read with tasks on goal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Name != "task-create" {
		t.Errorf("unexpected results: %+v", results)
	}

	// Error case in ExecuteQuery
	conn.queryResult = nil
	conn.queryErr = fmt.Errorf("cypher error")
	if _, err := st.FindCommands(ctx, "list commands"); err == nil {
		t.Error("expected error from FindCommands when ExecuteQuery fails")
	}

	// TraverseCommands fluent builder
	conn.queryErr = nil
	conn.queryResult = &provider.QueryResult{Nodes: []*provider.Node{node1}}
	traversal := st.TraverseCommands(ctx)
	resTrav, _ := traversal.For("tasks").
		For("goal").
		WithOperation("create").
		WithName("task").
		Execute()
	if len(resTrav) == 0 {
		t.Error("expected non-empty traversal results")
	}

	// Traversal with Rows
	conn.queryResult = &provider.QueryResult{
		Rows: []map[string]any{{"cmd": node1}},
	}
	resRows, err := st.TraverseCommands(ctx).For("tasks").Execute()
	if err != nil || len(resRows) == 0 {
		t.Errorf("expected rows traversal results: %v, %v", resRows, err)
	}

	// Traversal with error
	conn.queryErr = fmt.Errorf("traversal error")
	if _, err := st.TraverseCommands(ctx).Execute(); err == nil {
		t.Error("expected error on traversal failure")
	}

	// Nil connection on CommandTraversal
	nilTraversal := NewCommandTraversal(ctx, nil, reg)
	if _, err := nilTraversal.Execute(); err == nil {
		t.Error("expected error on nil connection traversal")
	}

	// Multiple parseQuery keywords
	conn.queryErr = nil
	conn.queryResult = &provider.QueryResult{Nodes: []*provider.Node{node1}}
	for _, q := range []string{"add task", "new goal", "show item", "search req", "modify bl", "change test", "remove crit", "del task"} {
		_, _ = st.FindCommands(ctx, q)
	}
}

func TestFileMetricsStore_Comprehensive(t *testing.T) {
	tempDir := t.TempDir()
	metricsPath := filepath.Join(tempDir, "metrics.json")
	chunksDir := filepath.Join(tempDir, "chunks")

	store, err := NewFileMetricsStoreWithConfig(metricsPath, chunksDir, 7)
	if err != nil {
		t.Fatalf("failed to create FileMetricsStore: %v", err)
	}
	if store.WindowDate() == "" {
		t.Error("expected non-empty WindowDate")
	}

	// Record metrics
	_ = store.RecordCommandExecution(&CommandMetric{Command: "test-cmd", NormalizedCmd: "test-cmd", Duration: 150 * time.Millisecond, Success: true})
	_ = store.RecordCommandExecution(&CommandMetric{Command: "test-fail", NormalizedCmd: "test-fail", Duration: 200 * time.Millisecond, Success: false})
	_ = store.RecordCommandExecution(&CommandMetric{Command: "test-timeout", NormalizedCmd: "test-timeout", Duration: 500 * time.Millisecond, TimedOut: true})
	store.WaitForFlushes()

	if err := store.Save(); err != nil {
		t.Fatalf("failed to save metrics: %v", err)
	}

	// Reload store
	store2, err := NewFileMetricsStore(metricsPath)
	if err != nil {
		t.Fatalf("failed to reload FileMetricsStore: %v", err)
	}
	m, err := store2.GetAllMetrics()
	if err != nil || len(m) < 3 {
		t.Errorf("expected at least 3 metrics, got %d, err: %v", len(m), err)
	}

	// Roll day
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	if err := store.RollDay(yesterday); err != nil {
		t.Fatalf("failed to RollDay: %v", err)
	}

	dates, err := store.ListRolledDates()
	if err != nil {
		t.Fatalf("failed to ListRolledDates: %v", err)
	}
	if len(dates) == 0 {
		t.Errorf("expected rolled dates, got none")
	}

	// Prune chunks and query historical
	_ = store.PruneOldChunks()
	_, _ = store.GetAllTimeMetrics()
	_, _ = store.GetMetricsSince(24 * time.Hour)
	_, _ = store.GetMetricsForDate(yesterday)
	_, _ = store.GetCommandMetrics("test-cmd")
}

type dummyHarnessSource struct {
	items []map[string]any
}

func (d *dummyHarnessSource) List(ctx context.Context, opts ListOptions) ([]map[string]any, int, error) {
	return d.items, len(d.items), nil
}

func TestHarness_ComprehensiveExtended(t *testing.T) {
	// ParseTripartiteIdentity
	a, b, c, err := ParseTripartiteIdentity("test:id:v1")
	if err != nil || a != "" || b != "" || c != "" {
		t.Logf("ParseTripartiteIdentity: %v, %v, %v, %v", a, b, c, err)
	}

	items := []map[string]any{
		{"id": "obj-1", "name": "Alpha", "score": 10, "tags": []any{"blue", "circle"}},
		{"id": "obj-2", "name": "Beta", "score": 30, "tags": []any{"red", "square"}},
		{"id": "obj-3", "name": "Alpha", "score": 20, "tags": []any{"blue", "triangle"}},
	}
	source := &dummyHarnessSource{items: items}

	// Run with sorting
	cmd := &cobra.Command{Use: "test"}
	AddListFlags(cmd)
	cfg := &ListConfig{
		ResolvedSet: &ListTraitSet{
			Listable:   true,
			Sortable:   true,
			Groupable:  true,
			Filterable: true,
			Formatable: true,
		},
	}

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)

	// Test sort desc
	_ = cmd.Flags().Set("sort-by", "score")
	_ = cmd.Flags().Set("sort-asc", "false")
	_ = cmd.Flags().Set("format", "yaml")
	if err := Run(ctx, cmd, source, cfg, &buf); err != nil {
		t.Fatalf("Run YAML sort failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Beta") {
		t.Errorf("expected YAML to contain Beta, got: %s", buf.String())
	}

	// Test group by
	buf.Reset()
	_ = cmd.Flags().Set("sort-by", "")
	_ = cmd.Flags().Set("group-by", "name")
	_ = cmd.Flags().Set("group-limit", "1")
	_ = cmd.Flags().Set("format", "json")
	if err := Run(ctx, cmd, source, cfg, &buf); err != nil {
		t.Fatalf("Run JSON group failed: %v", err)
	}

	// Test pagination & table
	buf.Reset()
	_ = cmd.Flags().Set("group-by", "")
	_ = cmd.Flags().Set("offset", "1")
	_ = cmd.Flags().Set("limit", "1")
	_ = cmd.Flags().Set("format", "table")
	if err := Run(ctx, cmd, source, cfg, &buf); err != nil {
		t.Fatalf("Run table paginate failed: %v", err)
	}

	// Test filter matching with slice
	buf.Reset()
	_ = cmd.Flags().Set("offset", "0")
	_ = cmd.Flags().Set("limit", "10")
	_ = cmd.Flags().Set("filter", "tags=blue")
	if err := Run(ctx, cmd, source, cfg, &buf); err != nil {
		t.Fatalf("Run filter slice failed: %v", err)
	}
}

func TestCommandSpecGraph_QueryCommandSpecs(t *testing.T) {
	ctx := context.Background()

	// Nil connection
	if _, err := QueryCommandSpecs(ctx, nil, CommandSpecFilter{}); err == nil {
		t.Error("expected error for nil connection")
	}

	node := &provider.Node{
		ID: "node-cmd",
		Properties: map[string]any{
			objects.FieldKeyName:       "object create",
			objects.FieldKeyType:       "cli",
			"operation_type":           "create",
			objects.FieldKeyTargetKind: "backlog_item",
			objects.FieldKeyFilePath:   "specs/object_create.yaml",
		},
	}
	conn := &queryableMockGraphConnection{
		queryResult: &provider.QueryResult{
			Nodes: []*provider.Node{node},
		},
	}

	filter := CommandSpecFilter{
		TargetKind:    "backlog_item",
		OperationType: "create",
		Name:          "create",
	}
	specs, err := QueryCommandSpecs(ctx, conn, filter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 || specs[0].Name != "object create" {
		t.Errorf("unexpected specs: %+v", specs)
	}
}

func TestCountOutput_AllKinds(t *testing.T) {
	counts := map[string]int{
		"backlog_item": 10,
		"criteria":     5,
		"goal":         2,
	}
	meta := map[string]any{"namespace_scope": "local"}

	for _, format := range []string{OutputFormatJSON, OutputFormatYAML, OutputFormatTable, "other"} {
		data, err := OutputAllKindsCount(counts, format, "scope note", meta)
		if err != nil {
			t.Errorf("OutputAllKindsCount(%s) failed: %v", format, err)
		}
		if len(data) == 0 {
			t.Errorf("OutputAllKindsCount(%s) produced empty output", format)
		}
	}
}

func TestDataLoader_FileAndStdin(t *testing.T) {
	tempDir := t.TempDir()
	logger := &dummyLoaderLogger{}

	// Non-existent file
	if _, _, err := LoadObjectDataFromFile(filepath.Join(tempDir, "none.yaml"), logger); err == nil {
		t.Error("expected error loading non-existent file")
	}

	// Corrupt file
	corruptPath := filepath.Join(tempDir, "corrupt.yaml")
	_ = fileutil.WriteFile(corruptPath, []byte(":\n:invalid"), paths.FilePerm644)
	if _, _, err := LoadObjectDataFromFile(corruptPath, logger); err == nil {
		t.Error("expected error loading corrupt YAML file")
	}

	// Valid file
	validPath := filepath.Join(tempDir, "valid.yaml")
	_ = fileutil.WriteFile(validPath, []byte("title: Hello\ncount: 42"), paths.FilePerm644)
	data, _, err := LoadObjectDataFromFile(validPath, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data["title"] != "Hello" {
		t.Errorf("unexpected data: %+v", data)
	}

	// LoadObjectDataFromString
	if _, _, err := LoadObjectDataFromString(":\n:invalid", logger); err == nil {
		t.Error("expected error loading invalid string")
	}
	dataStr, _, err := LoadObjectDataFromString("key: val", logger)
	if err != nil || dataStr["key"] != "val" {
		t.Errorf("unexpected string data: %+v, %v", dataStr, err)
	}

	// LoadObjectDataFromStdin (terminal returns error immediately)
	_, _, _ = LoadObjectDataFromStdin(logger)
}

func TestCommandSpecBuilder_Flags(t *testing.T) {
	tempDir := t.TempDir()

	// LoadCommandSpecYAML
	if _, err := LoadCommandSpecYAML(filepath.Join(tempDir, "missing.yaml")); err == nil {
		t.Error("expected error for missing YAML")
	}

	specPath := filepath.Join(tempDir, "spec.yaml")
	specYAML := `name: test-spec
use: test-spec [flags]
short: A test command
common_flags: true
flags:
  - name: title
    type: string
    description: Item title
  - name: count
    type: int
    default: 1
  - name: active
    type: bool
    default: false
  - name: timeout
    type: duration
    default: 5s
  - name: tags
    type: stringSlice
help:
  exclude_flags: ["timeout"]
subcommands:
  - name: sub1
    spec:
      name: sub1
      use: sub1
      flags:
        - name: extra
          type: string
`
	if err := fileutil.WriteFile(specPath, []byte(specYAML), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write spec YAML: %v", err)
	}

	spec, err := LoadCommandSpecYAML(specPath)
	if err != nil {
		t.Fatalf("failed to load CommandSpec: %v", err)
	}

	// ExpectedLocalFlagNamesFromCommandSpec
	names := ExpectedLocalFlagNamesFromCommandSpec(spec)
	if len(names) == 0 {
		t.Error("expected flags from command spec")
	}

	// BulkSubcommandSpec
	if _, ok := BulkSubcommandSpec(nil, "sub1"); ok {
		t.Error("expected false for nil parent")
	}
	if child, ok := BulkSubcommandSpec(spec, "sub1"); !ok || child == nil {
		t.Error("expected child spec for sub1")
	}
	if _, ok := BulkSubcommandSpec(spec, "nonexistent"); ok {
		t.Error("expected false for nonexistent subcommand")
	}

	// Test buildFlag
	builder := NewCommandSpecBuilder(spec)
	for _, f := range spec.Flags {
		cfg := builder.buildFlag(f)
		if cfg.Name != f.Name {
			t.Errorf("expected flag %s, got %s", f.Name, cfg.Name)
		}
	}
	crudSpec := &CRUDCommandSpec{
		CommandSpec:   *spec,
		OperationType: "create",
	}
	_ = spec.GetName()
	_ = crudSpec.GetName()
	_ = crudSpec.Validate()
	crudBuilder := NewCRUDCommandSpecBuilder(crudSpec).
		WithProcessor(nil).
		WithSpecsDir(tempDir)
	for _, f := range spec.Flags {
		cfg := crudBuilder.buildFlag(f)
		if cfg.Name != f.Name {
			t.Errorf("expected crud flag %s, got %s", f.Name, cfg.Name)
		}
	}
}

func TestCommandTracker_ComprehensiveExtended(t *testing.T) {
	tracker := NewCommandExecutionTracker()
	tracker.SetCommand("test-cmd", []string{"arg1"})
	tracker.SetNormalizedCommand("zqk object get")
	tracker.SetFlags(map[string]any{"flag": "val"})

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("str", "default", "")
	cmd.Flags().Bool("b", false, "")
	cmd.Flags().Int("i", 0, "")
	cmd.Flags().StringSlice("ss", nil, "")
	cmd.Flags().Duration("d", 0, "")

	_ = cmd.Flags().Set("str", "hello")
	_ = cmd.Flags().Set("b", "true")
	_ = cmd.Flags().Set("i", "42")
	_ = cmd.Flags().Set("ss", "one,two")
	_ = cmd.Flags().Set("d", "30s")

	extracted := ExtractFlagsFromCommand(cmd)
	if extracted["str"] != "hello" || extracted["b"] != true || extracted["i"] != 42 {
		t.Errorf("unexpected extracted flags: %+v", extracted)
	}

	norm := NormalizeCommand("zqk", []string{"object", "get", "BLI-1"})
	if !strings.Contains(norm, "zqk") {
		t.Errorf("unexpected normalized command: %q", norm)
	}
}

func TestTimeoutHook_TimeoutCalculation_Extended(t *testing.T) {
	hook := NewTimeoutHook()
	hook.SetMaxTimeout(10 * time.Minute)
	hook.SetCommandTimeout("zqk custom-action", 45*time.Second)

	// Command timeout match
	d := hook.getTimeoutForCommand("zqk custom-action", nil)
	if d != 45*time.Second {
		t.Errorf("expected 45s, got %v", d)
	}

	// MCP long-lived commands
	mcpD := hook.getTimeoutForCommand("zqk mcp serve", nil)
	if mcpD <= 0 {
		t.Errorf("expected positive duration for mcp serve, got %v", mcpD)
	}

	// With metrics store
	tempDir := t.TempDir()
	store, _ := NewFileMetricsStore(filepath.Join(tempDir, "metrics.json"))
	_ = store.RecordCommandExecution(&CommandMetric{Command: "zqk fast-cmd", Duration: 2 * time.Second, Success: true})
	hook.SetMetricsStore(store)

	fastD := hook.getTimeoutForCommand("zqk fast-cmd", nil)
	if fastD <= 0 {
		t.Errorf("expected calculated duration, got %v", fastD)
	}
	store.WaitForFlushes()
}

func TestMetricsAnalyzer_AnalyzeComprehensive(t *testing.T) {
	// Nil store error
	nilAnalyzer := NewMetricsAnalyzer(nil)
	if _, err := nilAnalyzer.Analyze(); err == nil {
		t.Error("expected error with nil store")
	}

	tempDir := t.TempDir()
	store, err := NewFileMetricsStore(filepath.Join(tempDir, "analyzer_metrics.json"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	// 5 executions for high-fail (ErrorRate = 100%)
	for i := 0; i < 5; i++ {
		_ = store.RecordCommandExecution(&CommandMetric{
			Command:       "zqk cmd-fail",
			NormalizedCmd: "zqk cmd-fail",
			Duration:      100 * time.Millisecond,
			Success:       false,
			Error:         "some error",
		})
	}
	// 5 executions for timeout (TimeoutRate = 100%)
	for i := 0; i < 5; i++ {
		_ = store.RecordCommandExecution(&CommandMetric{
			Command:       "zqk cmd-timeout",
			NormalizedCmd: "zqk cmd-timeout",
			Duration:      500 * time.Millisecond,
			TimedOut:      true,
		})
	}
	// 5 executions for slow command (AvgDuration > 30s)
	for i := 0; i < 5; i++ {
		_ = store.RecordCommandExecution(&CommandMetric{
			Command:       "zqk cmd-slow",
			NormalizedCmd: "zqk cmd-slow",
			Duration:      45 * time.Second,
			Success:       true,
		})
	}
	store.WaitForFlushes()

	analyzer := NewMetricsAnalyzer(store)
	res, err := analyzer.Analyze()
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	if len(res.HighFailureRateCommands) == 0 {
		t.Error("expected high failure rate commands")
	}
	if len(res.FrequentTimeouts) == 0 {
		t.Error("expected frequent timeout commands")
	}
	if len(res.SlowCommands) == 0 {
		t.Error("expected slow commands")
	}
	if len(res.ImprovementSuggestions) == 0 {
		t.Error("expected improvement suggestions")
	}

	report := analyzer.GenerateReport(res)
	if !strings.Contains(report, "Command Metrics Analysis") {
		t.Errorf("unexpected report output: %s", report)
	}
}

func TestHelpBuilder_DynamicAndSubcommands(t *testing.T) {
	cmd := &cobra.Command{Use: "parent-cmd", Short: "Parent command short"}
	subcmd := &cobra.Command{Use: "sub-cmd", Short: "Child subcommand short"}
	cmd.AddCommand(subcmd)
	cmd.Flags().String("opt1", "val", "Description of opt1")
	cmd.Flags().Bool("flag2", false, "Description of flag2")

	hb := StandardHelpBuilder("short", "desc line 1", "desc line 2")
	hb.WithAutoDiscoverSubcommands(true)
	hb.CategorizeFlag("Custom Category", "opt1")
	hb.WithIncludeCommonFlags(true)
	hb.setupDynamicSubcommandHelp(cmd)

	out := hb.BuildForCommand(cmd)
	if !strings.Contains(out, "sub-cmd") || !strings.Contains(out, "opt1") {
		t.Errorf("unexpected help text: %s", out)
	}

	objHB := ObjectCommandHelpBuilder("short", "backlog_item", "object command desc")
	objOut := objHB.BuildForCommand(cmd)
	if len(objOut) == 0 {
		t.Error("expected non-empty ObjectCommandHelpBuilder output")
	}

	dynHB := DynamicHelpBuilder("short", "dyn line")
	if dynOut := dynHB.BuildForCommand(cmd); len(dynOut) == 0 {
		t.Error("expected non-empty DynamicHelpBuilder output")
	}
}

func TestCodegen_GenerateFlagCodeAndArgs(t *testing.T) {
	tempDir := t.TempDir()
	specPath := filepath.Join(tempDir, "spec_codegen.yaml")
	specContent := `name: test-codegen
use: test-codegen [flags]
short: Short codegen description
flags:
  - name: bflag
    type: bool
    default: true
  - name: iflag
    type: int
    default: 10
  - name: fflag
    type: float
    default: 3.14
  - name: dflag
    type: duration
    default: 10s
  - name: saflag
    type: string_array
  - name: sflag
    type: string
    default: default_str
args:
  type: range
  min: 1
  max: 3
`
	if err := fileutil.WriteFile(specPath, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write spec: %v", err)
	}

	outputDir := filepath.Join(tempDir, "codegen_out")
	if err := GenerateCommandBuilderFromYAML(specPath, outputDir); err != nil {
		t.Fatalf("GenerateCommandBuilderFromYAML failed: %v", err)
	}

	genFile := filepath.Join(outputDir, defaultVersionPackage, "test_codegen_command_builder.go")
	content, err := fileutil.ReadFile(genFile)
	if err != nil {
		t.Fatalf("failed to read generated code: %v", err)
	}
	if !strings.Contains(string(content), "AddBoolFlag") || !strings.Contains(string(content), "AddDurationFlag") {
		t.Errorf("missing flag builders in generated code: %s", string(content))
	}
}

func TestUpdateDataLoader_FileAndDataExtended(t *testing.T) {
	tempDir := t.TempDir()
	logger := &dummyLoaderLogger{}

	// LoadUpdatesFromFile
	cmd := &cobra.Command{Use: "update"}
	cmd.Flags().String("file", filepath.Join(tempDir, "none.yaml"), "")
	if _, err := LoadUpdatesFromFile(cmd, logger); err == nil {
		t.Error("expected error for non-existent updates file")
	}

	corruptPath := filepath.Join(tempDir, "corrupt.yaml")
	_ = fileutil.WriteFile(corruptPath, []byte(":\n:invalid"), paths.FilePerm644)
	_ = cmd.Flags().Set("file", corruptPath)
	if _, err := LoadUpdatesFromFile(cmd, logger); err == nil {
		t.Error("expected error for corrupt updates file")
	}

	validPath := filepath.Join(tempDir, "valid_up.yaml")
	_ = fileutil.WriteFile(validPath, []byte("title: Updated Title\ncount: 99"), paths.FilePerm644)
	_ = cmd.Flags().Set("file", validPath)
	m, err := LoadUpdatesFromFile(cmd, logger)
	if err != nil || m["title"] != "Updated Title" {
		t.Fatalf("unexpected updates from file: %+v, %v", m, err)
	}

	// LoadUpdatesFromData
	cmdData := &cobra.Command{Use: "update"}
	cmdData.Flags().String("data", ":\n:invalid", "")
	if _, err := LoadUpdatesFromData(cmdData, logger); err == nil {
		t.Error("expected error for corrupt inline updates")
	}
	_ = cmdData.Flags().Set("data", "status: closed")
	dataUpdates, err := LoadUpdatesFromData(cmdData, logger)
	if err != nil || dataUpdates["status"] != "closed" {
		t.Fatalf("unexpected inline updates: %+v, %v", dataUpdates, err)
	}

	// LoadUpdatesFromStdin (terminal returns error immediately)
	_, _ = LoadUpdatesFromStdin(logger)

	// BuildUpdatesMap
	cmdCombined := &cobra.Command{Use: "update"}
	cmdCombined.Flags().StringArray("field", []string{"name=new_name", "active=true"}, "")
	cmdCombined.Flags().String("file", validPath, "")
	cmdCombined.Flags().String("data", "extra=extra_val", "")
	combined, err := BuildUpdatesMap(cmdCombined, logger)
	if err != nil {
		t.Fatalf("BuildUpdatesMap failed: %v", err)
	}
	if combined["name"] != "new_name" || combined["title"] != "Updated Title" {
		t.Errorf("unexpected combined updates: %+v", combined)
	}
}

func TestProfileLoader_CacheAndSearch(t *testing.T) {
	pl := NewProfileLoader("")
	pl.ClearCache()
	_ = findCLIProfilesDir()
}

func TestOverrideFriction_Comprehensive(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	ctx := context.Background()

	// Pulse stop test
	stop := pulseMeaningfulActivityWhileWaiting()
	stop()

	// Check 1: CI environment block
	t.Setenv("CI", "true")
	err := EnforceOverrideFriction(cmd, ctx, nil, nil, "OBJ-1", "backlog_item", "too short")
	if err == nil || !strings.Contains(err.Error(), "manual overrides are completely blocked in CI environments") {
		t.Errorf("expected CI block error, got: %v", err)
	}

	// Unset CI for remaining checks
	t.Setenv("CI", "")

	// Check 2: Reason code too short
	err = EnforceOverrideFriction(cmd, ctx, nil, nil, "OBJ-1", "backlog_item", "too short")
	if err == nil || !strings.Contains(err.Error(), "--reason-code must be a descriptive justification") {
		t.Errorf("expected reason code length error, got: %v", err)
	}

	// Non-TTY block check (test runs without a real terminal stdin)
	longReason := "This is a descriptive justification of at least thirty characters for override testing purposes."
	err = EnforceOverrideFriction(cmd, ctx, nil, nil, "OBJ-1", "backlog_item", longReason)
	if err == nil || !strings.Contains(err.Error(), "lifecycle --override is blocked without an interactive TTY") {
		t.Errorf("expected non-TTY error, got: %v", err)
	}
}

func TestCommandTraitValidation_Comprehensive(t *testing.T) {
	// Nil spec
	if err := ValidateCommandTraits(nil, "backlog_item"); err != nil {
		t.Errorf("expected nil for nil spec, got: %v", err)
	}

	spec := &CommandSpec{
		Name:           "test-cmd",
		RequiredTraits: []string{"lifecycle_entity"},
	}
	_ = ValidateCommandTraits(spec, "backlog_item")
	_ = ValidateCommandTraitsWithFlags(spec, "backlog_item", map[string]any{"flag": "val"})

	_ = GetCommandTraitRequirements(spec)
}

func TestCommandSpecCoverage_BaselineLoadWrite(t *testing.T) {
	tempDir := t.TempDir()
	baselinePath := filepath.Join(tempDir, "baseline.json")

	// Missing baseline
	if _, err := LoadCommandSpecCoverageBaseline(baselinePath); err == nil {
		t.Error("expected error for missing baseline")
	}

	// Write baseline with orphan spec should fail
	covWithOrphan := CommandSpecCoverage{
		SpecsWithoutCommands: []string{"orphan-spec"},
	}
	if err := WriteCommandSpecCoverageBaseline(baselinePath, covWithOrphan); err == nil {
		t.Error("expected error writing baseline with orphaned specs")
	}

	// Valid write
	covValid := CommandSpecCoverage{
		CommandsWithoutSpecs: []string{"cmd1", "cmd2"},
	}
	if err := WriteCommandSpecCoverageBaseline(baselinePath, covValid); err != nil {
		t.Fatalf("failed to write baseline: %v", err)
	}

	loaded, err := LoadCommandSpecCoverageBaseline(baselinePath)
	if err != nil {
		t.Fatalf("failed to load baseline: %v", err)
	}
	if len(loaded.CommandsWithoutSpecs) != 2 {
		t.Errorf("expected 2 commands without specs, got %d", len(loaded.CommandsWithoutSpecs))
	}
}

func TestCommandBuilderNaming_Comprehensive(t *testing.T) {
	// Numeric CAS stem checks
	if !IsNumericCASCommandStem("1785199714348041000_411cd659") {
		t.Error("expected true for numeric CAS stem")
	}
	if IsNumericCASCommandStem("custom_action") {
		t.Error("expected false for regular stem")
	}

	// IsProcessCommandSpecCAS
	if IsProcessCommandSpecCAS(nil) {
		t.Error("expected false for nil map")
	}
	if !IsProcessCommandSpecCAS(map[string]any{
		objects.FieldKeyKind: objects.KindCommandSpec,
		objects.FieldKeyID:   "CSPEC-12345",
	}) {
		t.Error("expected true for process command spec CAS")
	}

	// Nested path
	nestedName := ResolveCommandBuilderName(nil, "/path/to/cli/specs/object/list_command.yaml")
	if nestedName != "object_list" {
		t.Errorf("expected object_list, got %q", nestedName)
	}

	// CAS ID fallback
	casSpec := map[string]any{
		objects.FieldKeyID: "CSPEC-root-my_action_command",
	}
	casName := ResolveCommandBuilderName(casSpec, "foo.yaml")
	if casName != "my_action" {
		t.Errorf("expected my_action, got %q", casName)
	}
}

func TestFieldsOutput_FormatYAML(t *testing.T) {
	kf := &objects.KindFields{
		Kind: "test_kind",
	}
	yamlBytes, err := FormatKindFieldsSegregatedYAML(kf)
	if err != nil {
		t.Fatalf("failed to format segregated YAML: %v", err)
	}
	if len(yamlBytes) == 0 {
		t.Error("expected non-empty YAML bytes")
	}
}

func TestTimeoutHook_ResettableTimeoutComprehensive(t *testing.T) {
	hook := NewTimeoutHook()
	ctx := context.Background()

	// Direct setter/getter
	setTimeoutResetFunc(func() {})
	ResetTimeout()
	setServerContext(ctx)
	if sCtx := GetServerContext(); sCtx == nil {
		t.Error("expected non-nil server context")
	}

	// wrapCommandWithResettableTimeout with finite timeout
	ran := false
	err := hook.wrapCommandWithResettableTimeout(
		ctx, "test-server", "test-server", nil, nil,
		func() error {
			ran = true
			ResetTimeout()
			return nil
		},
		time.Now(), 100*time.Millisecond,
	)
	if err != nil || !ran {
		t.Errorf("expected successful execution, ran=%v, err=%v", ran, err)
	}

	// wrapCommandWithResettableTimeout with infinite timeout (0 or > 1000h)
	ranInf := false
	err = hook.wrapCommandWithResettableTimeout(
		ctx, "test-server-inf", "test-server-inf", nil, nil,
		func() error {
			ranInf = true
			return nil
		},
		time.Now(), 0,
	)
	if err != nil || !ranInf {
		t.Errorf("expected successful infinite timeout execution, ran=%v, err=%v", ranInf, err)
	}
}

func TestCountOutput_DirectAndMeta(t *testing.T) {
	metaFederated := map[string]any{
		"namespace_scope_mode": "federated",
	}
	metaLocal := map[string]any{
		"namespace_scope_mode": "local",
		"namespace_scope":      "zqk:test",
		"hidden_outside_scope": 5,
	}

	for _, format := range []string{OutputFormatJSON, OutputFormatYAML, OutputFormatTable, "text"} {
		_, err := OutputCountDirect(42, format, "backlog_item", metaFederated)
		if err != nil {
			t.Errorf("OutputCountDirect(%s) failed: %v", format, err)
		}
		_, err = OutputCountDirect(42, format, "backlog_item", metaLocal)
		if err != nil {
			t.Errorf("OutputCountDirect(%s) with local meta failed: %v", format, err)
		}
	}

	// namespaceLineFromMeta edge cases
	if line := namespaceLineFromMeta(nil); line != "" {
		t.Errorf("expected empty line for nil meta, got %q", line)
	}
	if line := namespaceLineFromMeta(map[string]any{"other": "val"}); line != "" {
		t.Errorf("expected empty line without mode, got %q", line)
	}
	if line := namespaceLineFromMeta(map[string]any{"namespace_scope_mode": "local"}); line != "" {
		t.Errorf("expected empty line without scope, got %q", line)
	}
}

func TestQueryFlags_ParseCountFlagsComprehensive(t *testing.T) {
	cmd := &cobra.Command{Use: "count"}
	AddCountFlags(cmd)

	logger := &dummyLoaderLogger{}

	// Successful parse
	_ = cmd.Flags().Set("filter", "kind=backlog_item")
	_ = cmd.Flags().Set("group-by", "status")
	filters, group, err := ParseCountFlags(cmd, logger, func(s string) (string, any, error) {
		parts := strings.SplitN(s, "=", 2)
		return parts[0], parts[1], nil
	})
	if err != nil || group != "status" || filters["kind"] != "backlog_item" {
		t.Errorf("unexpected count flags result: %v, %s, %v", filters, group, err)
	}

	// Error in filter parser
	_ = cmd.Flags().Set("filter", "invalid-filter")
	_, _, err = ParseCountFlags(cmd, logger, func(s string) (string, any, error) {
		return "", nil, fmt.Errorf("filter error")
	})
	if err == nil {
		t.Error("expected error for failed filter parse")
	}
}

func TestCRUDHelpers_ComprehensiveExtended(t *testing.T) {
	logger := &dummyLoaderLogger{}
	tempDir := t.TempDir()

	// isTempFilePath
	if !isTempFilePath("tmp-req.yaml") || !isTempFilePath("/dir/tmp_policy.yaml") {
		t.Error("expected true for tmp prefix")
	}
	if isTempFilePath("regular.yaml") || isTempFilePath("") {
		t.Error("expected false for non-tmp file")
	}

	// CleanupSourceFile
	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().Bool("keep-file", false, "")

	// Empty path
	CleanupSourceFile(cmd, "", logger)

	// Keep-file set
	_ = cmd.Flags().Set("keep-file", "true")
	CleanupSourceFile(cmd, "tmp-test.yaml", logger)

	// Actual cleanup
	_ = cmd.Flags().Set("keep-file", "false")
	tmpPath := filepath.Join(tempDir, "tmp-cleanup.yaml")
	_ = fileutil.WriteFile(tmpPath, []byte("data"), paths.FilePerm644)
	CleanupSourceFile(cmd, tmpPath, logger)
	if fileutil.Exists(tmpPath) {
		t.Error("expected tmp file to be deleted")
	}

	// FormatCreateSuccessMessage with and without ID
	msg1 := FormatCreateSuccessMessage(map[string]any{"id": "BLI-123"}, "backlog_item", "Backlog Item", logger)
	if !strings.Contains(msg1, "BLI-123") {
		t.Errorf("missing ID in success msg: %s", msg1)
	}
	msg2 := FormatCreateSuccessMessage(map[string]any{}, "backlog_item", "", logger)
	if !strings.Contains(msg2, "Object created successfully") {
		t.Errorf("missing object created in success msg: %s", msg2)
	}

	// FormatDeleteSuccessMessage
	delMsg := FormatDeleteSuccessMessage("BLI-123", false, false, "Backlog Item", logger)
	if !strings.Contains(delMsg, "BLI-123") {
		t.Errorf("missing ID in delete msg: %s", delMsg)
	}
}

func TestHarness_AdvancedFeatures(t *testing.T) {
	items := []map[string]any{
		{"id": "ID-1", "name": "One", "category": "A"},
		{"id": "ID-2", "name": "Two", "category": "B"},
		{"id": "ID-3", "name": "Three", "category": "A"},
	}
	source := &dummyHarnessSource{items: items}

	cfg := &ListConfig{
		ResolvedSet: &ListTraitSet{
			Listable:   true,
			Sortable:   true,
			Groupable:  true,
			Filterable: true,
			Formatable: true,
			CountOnly:  true,
		},
	}

	// 1. Test CountOnly (--count)
	cmdCount := &cobra.Command{Use: "list"}
	AddListFlags(cmdCount)
	_ = cmdCount.Flags().Set("count", "true")
	var bufCount bytes.Buffer
	ctx := context.Background()
	if err := Run(ctx, cmdCount, source, cfg, &bufCount); err != nil {
		t.Fatalf("Run with count failed: %v", err)
	}
	if strings.TrimSpace(bufCount.String()) != "3" {
		t.Errorf("expected count 3, got %q", bufCount.String())
	}

	// 2. Test IdsOnly (--ids-only) with JSON, YAML, Table
	for _, fmtStr := range []string{OutputFormatJSON, OutputFormatYAML, OutputFormatTable} {
		cmdIDs := &cobra.Command{Use: "list"}
		AddListFlags(cmdIDs)
		_ = cmdIDs.Flags().Set("ids-only", "true")
		_ = cmdIDs.Flags().Set("format", fmtStr)
		var bufIDs bytes.Buffer
		if err := Run(ctx, cmdIDs, source, cfg, &bufIDs); err != nil {
			t.Fatalf("Run with ids-only (%s) failed: %v", fmtStr, err)
		}
		if !strings.Contains(bufIDs.String(), "ID-1") {
			t.Errorf("expected ID-1 in ids-only output (%s), got %q", fmtStr, bufIDs.String())
		}
	}

	// 3. Test Grouped Table output
	cmdGroup := &cobra.Command{Use: "list"}
	AddListFlags(cmdGroup)
	_ = cmdGroup.Flags().Set("group-by", "category")
	_ = cmdGroup.Flags().Set("format", "table")
	var bufGroup bytes.Buffer
	if err := Run(ctx, cmdGroup, source, cfg, &bufGroup); err != nil {
		t.Fatalf("Run with grouped table failed: %v", err)
	}
	if !strings.Contains(bufGroup.String(), "=== A") || !strings.Contains(bufGroup.String(), "=== B") {
		t.Errorf("expected grouped headers in table, got: %s", bufGroup.String())
	}

	// 4. Test matchValue operators (not_equal, in, not_in)
	if !matchValue("foo", map[string]any{listFilterOpNotEqual: "bar"}) {
		t.Error("expected true for not_equal")
	}
	if !matchValue("foo", map[string]any{listFilterOpIn: []any{"foo", "baz"}}) {
		t.Error("expected true for in")
	}
	if !matchValue("foo", map[string]any{listFilterOpNotIn: []any{"bar", "baz"}}) {
		t.Error("expected true for not_in")
	}
	if matchValue("foo", map[string]any{listFilterOpIn: []any{"bar", "baz"}}) {
		t.Error("expected false for in with non-matching list")
	}
}

func TestDataLoader_LoadObjectDataWithFlags(t *testing.T) {
	logger := &dummyLoaderLogger{}

	// Test with --data and --field
	cmd := &cobra.Command{Use: "create"}
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("data", "title: OriginalTitle", "")
	cmd.Flags().StringArray("field", []string{"title=UpdatedTitle", "count=10"}, "")

	data, _, err := LoadObjectData(cmd, logger)
	if err != nil {
		t.Fatalf("LoadObjectData failed: %v", err)
	}
	if data["title"] != "UpdatedTitle" || fmt.Sprint(data["count"]) != "10" {
		t.Errorf("unexpected loaded data: %+v", data)
	}

	// Test with no data (empty inputs -> error)
	cmdEmpty := &cobra.Command{Use: "create"}
	cmdEmpty.Flags().String("file", "", "")
	cmdEmpty.Flags().String("data", "", "")
	cmdEmpty.Flags().StringArray("field", nil, "")
	_, _, err = LoadObjectData(cmdEmpty, logger)
	if err == nil {
		t.Error("expected error when no data is provided")
	}
}

func TestTimeoutHook_LifecycleWait(t *testing.T) {
	hook := NewTimeoutHook()
	hook.Wait()
	hook.SetMetricsStore(nil)
	globalHook := GetTimeoutHook()
	if globalHook == nil {
		t.Error("expected non-nil global timeout hook")
	}
}

func TestCompletions_Comprehensive(t *testing.T) {
	// Nil index
	if candidates := BuildFieldCompletionCandidates(nil, "backlog_item", nil); candidates != nil {
		t.Error("expected nil for nil index")
	}
	if enums := BuildEnumValueCompletions(nil, "backlog_item", "status", ""); enums != nil {
		t.Error("expected nil for nil index in enum completions")
	}

	idx := &objects.SpecIndex{
		Kinds: map[string]objects.SpecKindSummary{
			"backlog_item": {
				Kind: "backlog_item",
				Fields: []objects.SpecFieldSummary{
					{
						Name:         "status",
						Type:         "string",
						SemanticType: "enum",
						EnumValues:   []string{"draft", "active", "completed", "cancelled"},
						Filterable:   true,
						Groupable:    true,
					},
					{
						Name:     "score",
						Type:     "int",
						Numeric:  true,
						MinValue: 1,
						MaxValue: 100,
						Sortable: true,
					},
					{
						Name:          "title",
						Type:          "string",
						MinLength:     3,
						MaxLength:     80,
						DisplayLength: 40,
						Filterable:    true,
					},
					{
						Name:     "created_at",
						Type:     "string",
						TimeLike: true,
						Format:   "date-time",
					},
					{
						Name:   "misc",
						Traits: []string{"audit_only"},
					},
				},
			},
		},
	}

	candidates := BuildFieldCompletionCandidates(idx, "backlog_item", func(f objects.SpecFieldSummary) bool {
		return f.Filterable
	})
	if len(candidates) != 2 {
		t.Errorf("expected 2 candidates, got %d", len(candidates))
	}

	enums := BuildEnumValueCompletions(idx, "backlog_item", "status", "act")
	if len(enums) != 1 || enums[0] != "active" {
		t.Errorf("unexpected enums: %v", enums)
	}

	// Unknown kind
	if enumsNone := BuildEnumValueCompletions(idx, "unknown_kind", "status", ""); enumsNone != nil {
		t.Errorf("expected nil for unknown kind, got %v", enumsNone)
	}
}

func TestIDLoader_Comprehensive(t *testing.T) {
	tempDir := t.TempDir()
	logger := &dummyLoaderLogger{}

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("ids", "", "")
	cmd.Flags().String("file", "", "")

	// Neither flag -> error
	if _, err := LoadIDsFromFlags(cmd, logger); err == nil {
		t.Error("expected error when neither flag is set")
	}

	// From ids flag
	_ = cmd.Flags().Set("ids", "ID-1, ID-2, ID-3")
	ids, err := LoadIDsFromFlags(cmd, logger)
	if err != nil || len(ids) != 3 {
		t.Errorf("unexpected ids from flag: %v, %v", ids, err)
	}

	// From file flag
	_ = cmd.Flags().Set("ids", "")
	idsPath := filepath.Join(tempDir, "ids.yaml")
	_ = fileutil.WriteFile(idsPath, []byte("- ID-10\n- ID-20\n"), paths.FilePerm644)
	_ = cmd.Flags().Set("file", idsPath)
	idsFromFile, err := LoadIDsFromFlags(cmd, logger)
	if err != nil || len(idsFromFile) != 2 {
		t.Errorf("unexpected ids from file: %v, %v", idsFromFile, err)
	}

	// Corrupt file
	corruptPath := filepath.Join(tempDir, "corrupt_ids.yaml")
	_ = fileutil.WriteFile(corruptPath, []byte(":\n:invalid"), paths.FilePerm644)
	_ = cmd.Flags().Set("file", corruptPath)
	if _, err := LoadIDsFromFlags(cmd, logger); err == nil {
		t.Error("expected error for corrupt ids file")
	}
}

func TestProfileLoader_Extended(t *testing.T) {
	tempDir := t.TempDir()
	pl := NewProfileLoader(tempDir)

	// Profile not found
	if _, err := pl.LoadProfile("nonexistent"); err == nil {
		t.Error("expected error for nonexistent profile")
	}

	// Invalid namespace
	badNSPath := filepath.Join(tempDir, "bad_ns.yaml")
	_ = fileutil.WriteFile(badNSPath, []byte("namespace_id: invalid:ns\n"), paths.FilePerm644)
	if _, err := pl.LoadProfile("bad_ns"); err == nil {
		t.Error("expected error for invalid namespace")
	}

	// Parent profile
	parentPath := filepath.Join(tempDir, "parent.yaml")
	parentContent := `namespace_id: ` + paths.CLINamespaceID + `
spec:
  key1: val1
  nested:
    p_key: p_val
`
	_ = fileutil.WriteFile(parentPath, []byte(parentContent), paths.FilePerm644)

	// Child profile extending parent
	childPath := filepath.Join(tempDir, "child.yaml")
	childContent := `namespace_id: ` + paths.CLINamespaceID + `
extends: parent
spec:
  key2: val2
  nested:
    c_key: c_val
`
	_ = fileutil.WriteFile(childPath, []byte(childContent), paths.FilePerm644)

	prof, err := pl.LoadProfile("child")
	if err != nil {
		t.Fatalf("LoadProfile(child) failed: %v", err)
	}
	if prof.ResolvedSpec["key1"] != "val1" || prof.ResolvedSpec["key2"] != "val2" {
		t.Errorf("unexpected resolved spec: %+v", prof.ResolvedSpec)
	}

	// Circular inheritance
	circ1Path := filepath.Join(tempDir, "circ1.yaml")
	circ2Path := filepath.Join(tempDir, "circ2.yaml")
	_ = fileutil.WriteFile(circ1Path, []byte("extends: circ2\n"), paths.FilePerm644)
	_ = fileutil.WriteFile(circ2Path, []byte("extends: circ1\n"), paths.FilePerm644)
	if _, err := pl.LoadProfile("circ1"); err == nil {
		t.Error("expected error for circular inheritance")
	}
}
