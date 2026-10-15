package newcmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestEmitJITGuidanceTip_AllKinds(t *testing.T) {
	kinds := []string{
		objects.KindGoal,
		objects.KindRequirement,
		objects.KindMilestone,
		objects.KindTestCase,
		objects.KindBacklogItem,
		objects.KindPriorityPlan,
		objects.KindEpic,
		objects.KindTechnicalDebt,
		"unknown_kind",
	}

	for _, kind := range kinds {
		var buf bytes.Buffer
		cmd := &cobra.Command{Use: "test"}
		cmd.SetErr(&buf)
		emitJITGuidanceTip(cmd, kind)
		out := buf.String()
		if kind != "unknown_kind" && !strings.Contains(out, "Shift-Left Tip") {
			t.Errorf("kind %s: expected guidance tip, got %q", kind, out)
		}
	}

	// JSON/YAML format suppress tip
	formats := []string{"json", "yaml", "stream"}
	for _, f := range formats {
		var buf bytes.Buffer
		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("format", "", "")
		_ = cmd.Flags().Set("format", f)
		cmd.SetErr(&buf)
		emitJITGuidanceTip(cmd, objects.KindGoal)
		if buf.Len() != 0 {
			t.Errorf("format %s: expected empty tip, got %q", f, buf.String())
		}
	}
}

func TestReadTitleAndBody_Paths(t *testing.T) {
	// 1. Direct content parsing
	body, title, err := readTitleAndBody("", "# Direct Title\n\nDirect body content here", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "Direct Title" {
		t.Errorf("expected 'Direct Title', got %q", title)
	}
	if !strings.Contains(body, "Direct body content here") {
		t.Errorf("expected body to contain content, got %q", body)
	}

	// 2. Title override
	body, title, err = readTitleAndBody("", "# Old Title\n\nBody", "New Title Override")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "New Title Override" {
		t.Errorf("expected 'New Title Override', got %q", title)
	}

	// 3. File reading
	tmpFile := filepath.Join(t.TempDir(), "draft.md")
	if err := fileutil.WriteFile(tmpFile, []byte("# From File\n\nBody from file."), paths.FilePerm644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	body, title, err = readTitleAndBody(tmpFile, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if title != "From File" {
		t.Errorf("expected 'From File', got %q", title)
	}
	if !strings.Contains(body, "Body from file.") {
		t.Errorf("expected body from file, got %q", body)
	}

	// 4. File reading error
	_, _, err = readTitleAndBody(filepath.Join(t.TempDir(), "nonexistent.md"), "", "")
	if err == nil {
		t.Errorf("expected error for nonexistent file")
	}
}

func TestEnqueueMintPromoteJob(t *testing.T) {
	// Empty inputs fail-closed
	_, err := enqueueMintPromoteJob("", "BLI-123")
	if err == nil {
		t.Error("expected error with empty projectRoot")
	}
	_, err = enqueueMintPromoteJob("/tmp/repo", "")
	if err == nil {
		t.Error("expected error with empty objectID")
	}

	// Valid job creation with isolated project
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)
	repoRoot := zqkenv.ProjectRoot().Get()
	if repoRoot == "" {
		if root, err := findModuleRoot(); err == nil {
			repoRoot = root
		}
	}
	if err := testenvroot.BootstrapRoot(testRoot, repoRoot); err != nil {
		t.Fatalf("BootstrapRoot: %v", err)
	}
	defer func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil))
	}()

	jobID, err := enqueueMintPromoteJob(testRoot, "BLI-TEST-MINT-001")
	if err != nil {
		t.Fatalf("enqueueMintPromoteJob: %v", err)
	}
	if !strings.HasPrefix(jobID, "SCH-mint-promote-") {
		t.Errorf("expected prefix SCH-mint-promote-, got %q", jobID)
	}
}

func TestNewObject_EdgeCases(t *testing.T) {
	testRoot := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), testRoot)
	repoRoot := zqkenv.ProjectRoot().Get()
	if repoRoot == "" {
		if root, err := findModuleRoot(); err == nil {
			repoRoot = root
		}
	}
	if err := testenvroot.BootstrapRoot(testRoot, repoRoot); err != nil {
		t.Fatalf("BootstrapRoot: %v", err)
	}
	defer func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil))
	}()

	// Test object command with --file
	tmpFile := filepath.Join(testRoot, "spec.md")
	if err := fileutil.WriteFile(tmpFile, []byte("# Markdown Title\nSpec body"), paths.FilePerm644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd := NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"object", "backlog_item", "--file", tmpFile})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("object with --file failed: %v", err)
	}

	// Test object command with --content
	buf.Reset()
	cmd = NewNewCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"object", "backlog_item", "--content", "# Inline Title\nInline body"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("object with --content failed: %v", err)
	}
}
