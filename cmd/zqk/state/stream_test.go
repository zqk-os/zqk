package state

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestStateCommands(t *testing.T) {
	t.Parallel()

	cmd := NewStateCmd()
	if cmd.Use != "state" {
		t.Fatalf("expected command use 'state', got %q", cmd.Use)
	}

	foundTree := false
	foundJournal := false
	foundStream := false

	for _, sub := range cmd.Commands() {
		switch sub.Name() {
		case "tree":
			foundTree = true
		case "journal":
			foundJournal = true
		case "stream":
			foundStream = true
		}
	}

	if !foundTree {
		t.Error("expected 'tree' subcommand on state command")
	}
	if !foundJournal {
		t.Error("expected 'journal' subcommand on state command")
	}
	if !foundStream {
		t.Error("expected 'stream' subcommand on state command")
	}
}

func TestStateStreamCommand(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	streamDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.StreamsDir, "change_journal_entry")
	if err := fileutil.MkdirAll(streamDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create test stream dir: %v", err)
	}

	m1 := JournalMutation{
		ID:          "CHA-TEST-100",
		ChangeType:  "create",
		ObjectRef:   "backlog_item:BLI-001",
		DiffSummary: "Initial creation",
		CreatedAt:   time.Now().Unix(),
		CreatedBy:   "ACC-TEST",
	}
	m2 := JournalMutation{
		ID:          "CHA-TEST-101",
		ChangeType:  "promote",
		ObjectRef:   "backlog_item:BLI-001",
		DiffSummary: "promoted to in_progress",
		CreatedAt:   time.Now().Unix() + 1,
		CreatedBy:   "ACC-TEST",
	}

	b1, _ := json.Marshal(m1)
	b2, _ := json.Marshal(m2)
	content := string(b1) + "\n" + string(b2) + "\n"

	targetFile := filepath.Join(streamDir, "test_journal.json")
	if err := fileutil.WriteFile(targetFile, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test journal: %v", err)
	}

	// 1. Test Static Text Formatting
	summaryOut := BuildStreamSummary([]JournalMutation{m1, m2})
	if !strings.Contains(summaryOut, "CHA-TEST-100") {
		t.Errorf("expected CHA-TEST-100 in output, got: %s", summaryOut)
	}
	if !strings.Contains(summaryOut, "CHA-TEST-101") {
		t.Errorf("expected CHA-TEST-101 in output, got: %s", summaryOut)
	}
	if !strings.Contains(summaryOut, "CPCP: PASS") {
		t.Errorf("expected CPCP: PASS in output, got: %s", summaryOut)
	}

	line1 := FormatMutationLine(m1)
	if !strings.Contains(line1, "CREATE") || !strings.Contains(line1, "BLI-001") {
		t.Errorf("unexpected FormatMutationLine output: %s", line1)
	}

	// 2. Test Stream Execution (Static Mode)
	testCmd := &cobra.Command{}
	err := StreamJournalMutations(testCmd, tempDir, false, 10, "")
	if err != nil {
		t.Fatalf("StreamJournalMutations failed: %v", err)
	}

	// 3. Test Stream Execution (JSON Mode)
	jsonCmd := &cobra.Command{}
	err = StreamJournalMutations(jsonCmd, tempDir, false, 10, "json")
	if err != nil {
		t.Fatalf("StreamJournalMutations json failed: %v", err)
	}

	// 4. Test Follow with Context Cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	followCmd := &cobra.Command{}
	followCmd.SetContext(ctx)

	start := time.Now()
	err = StreamJournalMutations(followCmd, tempDir, true, 10, "")
	if err != nil {
		t.Fatalf("StreamJournalMutations follow failed: %v", err)
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Errorf("expected follow to wait for context cancellation, exited too early: %v", time.Since(start))
	}

	// 5. Test Visual Seismograph Dashboard Formatting
	dashOut := BuildDashboardView(tempDir, []JournalMutation{m1, m2})
	if !strings.Contains(dashOut, "ZQK STATE SEISMOGRAPH & TELEMETRY DASHBOARD") {
		t.Errorf("expected dashboard title banner, got:\n%s", dashOut)
	}
	if !strings.Contains(dashOut, "CPCP-MEMBRANE-001") {
		t.Errorf("expected CPCP-MEMBRANE-001 in dashboard, got:\n%s", dashOut)
	}
	if !strings.Contains(dashOut, "BLI-001") {
		t.Errorf("expected BLI-001 in dashboard table, got:\n%s", dashOut)
	}

	// 6. Test Stream Execution (Dashboard Mode)
	dashCmd := &cobra.Command{}
	err = StreamJournalMutations(dashCmd, tempDir, false, 10, "", true)
	if err != nil {
		t.Fatalf("StreamJournalMutations dashboard failed: %v", err)
	}

	// 7. Test Follow with Dashboard Mode Context Cancellation
	dashCtx, dashCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer dashCancel()

	followDashCmd := &cobra.Command{}
	followDashCmd.SetContext(dashCtx)

	err = StreamJournalMutations(followDashCmd, tempDir, true, 10, "", true)
	if err != nil {
		t.Fatalf("StreamJournalMutations follow dashboard failed: %v", err)
	}
}
