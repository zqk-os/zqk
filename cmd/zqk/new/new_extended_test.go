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
	t.Cleanup(func() {
		if teardownErr := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil)); teardownErr != nil {
			t.Logf("teardown error: %v", teardownErr)
		}
	})

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
	t.Cleanup(func() {
		if teardownErr := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil)); teardownErr != nil {
			t.Logf("teardown error: %v", teardownErr)
		}
	})

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

func TestNewObject_AgentTaskEstimatedEffort(t *testing.T) {
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
	t.Cleanup(func() {
		if teardownErr := storage.RunProjectTestTeardown(storage.TempProjectTeardown(testRoot, nil)); teardownErr != nil {
			t.Logf("teardown error: %v", teardownErr)
		}
	})

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)

	// 1. Explicit --estimated-effort 4h
	cmd1 := NewNewCmd()
	cmd1.SetContext(ctx)
	cmd1.SetArgs([]string{"object", "agent_task", "--title", "Explicit Effort Task", "-d", "Test agent task description 1", "--estimated-effort", "4h", "--cas"})
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("agent_task with --estimated-effort failed: %v", err)
	}

	// 2. Explicit --effort 30m alias
	buf.Reset()
	cmd2 := NewNewCmd()
	cmd2.SetContext(ctx)
	cmd2.SetArgs([]string{"object", "agent_task", "--title", "Effort Alias Task", "-d", "Test agent task description 2", "--effort", "30m", "--cas"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("agent_task with --effort failed: %v", err)
	}

	// 3. Default effort fallback for agent_task (2h)
	buf.Reset()
	cmd3 := NewNewCmd()
	cmd3.SetContext(ctx)
	cmd3.SetArgs([]string{"object", "agent_task", "--title", "Default Effort Task", "-d", "Test agent task description 3", "--cas"})
	if err := cmd3.Execute(); err != nil {
		t.Fatalf("agent_task with default effort failed: %v", err)
	}

	sp, err := storage.GetGlobalStorageProviderCache().GetOrCreate(ctx, testRoot)
	if err != nil {
		t.Fatalf("storage cache: %v", err)
	}
	sec := pkgctx.NewSystemSecurityContext()
	res, err := sp.List(ctx, sec, nil, storage.ListFilter{Kind: objects.KindAgentTask})
	if err != nil {
		t.Fatalf("list agent tasks: %v", err)
	}

	effortByTitle := make(map[string]string)
	for _, obj := range res.Objects {
		title, _ := obj[objects.FieldKeyTitle].(string)
		effort, _ := obj[objects.FieldKeyEstimatedEffort].(string)
		effortByTitle[title] = effort
	}

	if effortByTitle["Explicit Effort Task"] != "4h" {
		t.Errorf("expected 4h for Explicit Effort Task, got %q", effortByTitle["Explicit Effort Task"])
	}
	if effortByTitle["Effort Alias Task"] != "30m" {
		t.Errorf("expected 30m for Effort Alias Task, got %q", effortByTitle["Effort Alias Task"])
	}
	if effortByTitle["Default Effort Task"] != "2h" {
		t.Errorf("expected 2h for Default Effort Task, got %q", effortByTitle["Default Effort Task"])
	}
}
