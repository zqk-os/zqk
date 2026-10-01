package system

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestMetricsCmd_Flags(t *testing.T) {
	cmd := NewMetricsCmd()
	if cmd == nil {
		t.Fatal("expected command, got nil")
	}

	for _, flag := range []string{"command", "filter", "limit", "summary", "all-time", "window", "day"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag --%s", flag)
		}
	}
}

func TestMetricsCmd_ExecutionWithDayRoll(t *testing.T) {
	tmpDir := t.TempDir()
	metricsDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.MetricsDir)
	chunksDir := filepath.Join(metricsDir, paths.MetricsCommandMetricsSubdir)
	if err := fileutil.MkdirAll(chunksDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	today := time.Now().UTC().Format("2006-01-02")
	yesterday := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	yesterdayCompact := strings.ReplaceAll(yesterday, "-", "")

	// 1. Write active command_metrics.json for today
	todayMetrics := map[string]*clipkg.CommandMetrics{
		"zqk system check": {
			Command:         "zqk",
			NormalizedCmd:   "zqk system check",
			InvocationCount: 3,
			SuccessCount:    3,
			AvgDuration:     100 * time.Millisecond,
		},
	}
	todayData := struct {
		Metrics    map[string]*clipkg.CommandMetrics `json:"metrics"`
		Updated    time.Time                         `json:"updated"`
		WindowDate string                            `json:"window_date"`
	}{
		Metrics:    todayMetrics,
		Updated:    time.Now().UTC(),
		WindowDate: today,
	}
	bToday, _ := json.MarshalIndent(todayData, "", "  ")
	_ = fileutil.WriteFile(filepath.Join(metricsDir, paths.CommandMetricsFile), bToday, paths.FilePerm644)

	// 2. Write rolled chunk for yesterday
	yesterdayMetrics := map[string]*clipkg.CommandMetrics{
		"zqk object get": {
			Command:         "zqk",
			NormalizedCmd:   "zqk object get",
			InvocationCount: 10,
			SuccessCount:    8,
			FailureCount:    2,
			ErrorRate:       20.0,
			AvgDuration:     50 * time.Millisecond,
		},
	}
	yesterdayData := struct {
		Metrics    map[string]*clipkg.CommandMetrics `json:"metrics"`
		Updated    time.Time                         `json:"updated"`
		WindowDate string                            `json:"window_date"`
	}{
		Metrics:    yesterdayMetrics,
		Updated:    time.Now().UTC().Add(-24 * time.Hour),
		WindowDate: yesterday,
	}
	bYesterday, _ := json.MarshalIndent(yesterdayData, "", "  ")
	yesterdayChunkFile := filepath.Join(chunksDir, fmt.Sprintf("command_metrics_%s.json", yesterdayCompact))
	_ = fileutil.WriteFile(yesterdayChunkFile, bYesterday, paths.FilePerm644)

	// Test default (today's bounded window): should show zqk system check
	cmd := NewMetricsCmd()
	var outBuf bytes.Buffer
	cmd.SetArgs([]string{"--format", "json"})
	cmdCtx := cli.ContextForProjectAndProfile(tmpDir, "test")
	cli.SetContext(cmd, cmdCtx)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &outBuf))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	out := outBuf.String()
	if !strings.Contains(out, "zqk system check") {
		t.Errorf("expected output to contain 'zqk system check', got: %s", out)
	}
	if strings.Contains(out, "zqk object get") {
		t.Errorf("expected bounded today's output NOT to contain yesterday's 'zqk object get', got: %s", out)
	}

	// Test --day <yesterday>: should show yesterday's zqk object get
	cmdDay := NewMetricsCmd()
	var outBufDay bytes.Buffer
	cmdDay.SetArgs([]string{"--day", yesterday, "--format", "json"})
	cli.SetContext(cmdDay, cmdCtx)
	cmdDay.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &outBufDay))

	if err := cmdDay.Execute(); err != nil {
		t.Fatalf("execute --day failed: %v", err)
	}

	outDay := outBufDay.String()
	if !strings.Contains(outDay, "zqk object get") {
		t.Errorf("expected --day output to contain 'zqk object get', got: %s", outDay)
	}

	// Test --all-time: should contain BOTH
	cmdAll := NewMetricsCmd()
	var outBufAll bytes.Buffer
	cmdAll.SetArgs([]string{"--all-time", "--format", "json"})
	cli.SetContext(cmdAll, cmdCtx)
	cmdAll.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &outBufAll))

	if err := cmdAll.Execute(); err != nil {
		t.Fatalf("execute --all-time failed: %v", err)
	}

	outAll := outBufAll.String()
	if !strings.Contains(outAll, "zqk system check") || !strings.Contains(outAll, "zqk object get") {
		t.Errorf("expected --all-time output to contain both commands, got: %s", outAll)
	}

	// Test --window 48h: should contain yesterday's zqk object get and today's zqk system check
	cmdWin := NewMetricsCmd()
	var outBufWin bytes.Buffer
	cmdWin.SetArgs([]string{"--window", "48h", "--format", "json"})
	cli.SetContext(cmdWin, cmdCtx)
	cmdWin.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &outBufWin))

	if err := cmdWin.Execute(); err != nil {
		t.Fatalf("execute --window failed: %v", err)
	}

	outWin := outBufWin.String()
	if !strings.Contains(outWin, "zqk system check") || !strings.Contains(outWin, "zqk object get") {
		t.Errorf("expected --window 48h output to contain both commands, got: %s", outWin)
	}
}
