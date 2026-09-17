package system

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/workflow/whatsnext"
)

func TestMetricsFeedCmd(t *testing.T) {
	cmd := NewMetricsFeedCmd()
	if cmd == nil {
		t.Fatal("expected command, got nil")
	}

	if cmd.Use != "feed" {
		t.Errorf("expected Use 'feed', got %q", cmd.Use)
	}

	// Basic flag checks
	if cmd.Flags().Lookup("agent-id") == nil {
		t.Error("expected --agent-id flag")
	}
	if cmd.Flags().Lookup("notify") == nil {
		t.Error("expected --notify flag")
	}

	// Test output buffering and help
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("help execution failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Analyzes command metrics and pushes a digest to agent_feed") {
		t.Errorf("expected help output to contain command description, got: %s", output)
	}
}

func TestParseHumanLogClusters(t *testing.T) {
	tmpDir := t.TempDir()
	logDir := filepath.Join(tmpDir, paths.ProjectDataDir, paths.LogsDir)
	if err := fileutil.MkdirAll(logDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	humanLogPath := filepath.Join(logDir, paths.LogEventsPrefix+"human.log")
	logContent := `2026-08-13T04:17:33Z [info] Validating lifecycle event=info
2026-08-13T04:17:33Z [error] Database connection failed attempt=1
2026-08-13T04:17:34Z [error] Database connection failed attempt=2
2026-08-13T04:17:35Z [warn] High memory usage percent=90
2026-08-13T04:17:36Z [error] Unexpected EOF
`
	if err := fileutil.WriteFile(humanLogPath, []byte(logContent), 0644); err != nil {
		t.Fatal(err)
	}

	result := whatsnext.FormatHumanLogClusterDigest(whatsnext.ParseHumanLogClusters(tmpDir))
	if !strings.Contains(result, "Database connection failed (2)") {
		t.Errorf("expected 'Database connection failed (2)' in result, got %q", result)
	}
	if !strings.Contains(result, "Unexpected EOF (1)") {
		t.Errorf("expected 'Unexpected EOF (1)' in result, got %q", result)
	}
	if !strings.Contains(result, "High memory usage (1)") {
		t.Errorf("expected 'High memory usage (1)' in result, got %q", result)
	}
}
